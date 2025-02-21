package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"

	"embed"

	"endobit.io/metal"
	pb "endobit.io/metal/gen/go/proto/metal/v1"
)

//go:embed reports/*.tmpl
var reports embed.FS

type Reporter struct {
	Client      *metal.Client
	Logger      *slog.Logger
	initialized bool
	tmpl        *template.Template
}

type reportScope struct {
	Zone    string
	Cluster string
	Host    string
}

// ServeHTTP implements the http.Handler interface.
func (r *Reporter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if !r.initialized {
		if err := r.init(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	q := req.URL.Query()

	scope := reportScope{
		Zone:    q.Get("zone"),
		Cluster: q.Get("cluster"),
		Host:    q.Get("host"),
	}

	name := req.PathValue("name")

	b, err := r.report(scope, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if _, err := w.Write(b); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (r *Reporter) report(scope reportScope, name string) ([]byte, error) {
	ctx := r.Client.Context() // grabs the token

	var req pb.ReadReportDataRequest

	if scope.Zone != "" {
		req.SetZone(scope.Zone)
	}
	if scope.Cluster != "" {
		req.SetCluster(scope.Cluster)
	}
	if scope.Host != "" {
		req.SetHost(scope.Host)
	}

	resp, err := r.Client.Metal.ReadReportData(ctx, &req)
	if err != nil {
		return nil, err
	}

	var data metal.ReportData

	if err := json.Unmarshal(resp.GetData(), &data); err != nil {
		return nil, err
	}

	var buf bytes.Buffer

	if err := r.tmpl.ExecuteTemplate(&buf, name+".tmpl", data); err != nil {
		return nil, fmt.Errorf("failed to execute template %q: %w", name, err)
	}

	return buf.Bytes(), nil
}

func (r *Reporter) init() error {
	tmpl := template.New("mops")

	funcs := sprig.TxtFuncMap()
	funcs["include"] = func(name string, data interface{}) (string, error) {
		var buf strings.Builder

		err := tmpl.ExecuteTemplate(&buf, name, data)
		return buf.String(), err
	}

	repfs, err := fs.Sub(reports, "reports")
	if err != nil {
		return err
	}

	if err := logFiles(repfs, r.Logger.WithGroup("templates")); err != nil {
		return err
	}

	tmpl, err = tmpl.Funcs(funcs).ParseFS(repfs, "*.tmpl")
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	r.tmpl = tmpl
	r.initialized = true

	return nil
}

// logFiles recursively logs every file and directory in the provided filesystem using the provided logger.
func logFiles(fsys fs.FS, logger *slog.Logger) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			logger.Info("found", "file", path)
		}
		return nil
	})
}
