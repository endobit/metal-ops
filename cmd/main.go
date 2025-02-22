package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"endobit.io/metal"
	ops "endobit.io/metal-ops"
	"endobit.io/metal-ops/internal/handlers"
	authpb "endobit.io/metal/gen/go/proto/auth/v1"
	metalpb "endobit.io/metal/gen/go/proto/metal/v1"
	"endobit.io/metal/logging"
)

var version string

func main() {
	cmd := newRootCmd()
	cmd.Version = version

	if err := cmd.Execute(); err != nil {
		os.Exit(-1)
	}
}

func newRootCmd() *cobra.Command {
	var (
		username, password, metalServer string
		port                            int
		logOpts                         *logging.Options
	)

	cmd := cobra.Command{
		Use:   "mopsd",
		Short: "Metal Operations Server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger, err := logOpts.NewLogger()
			if err != nil {
				return err
			}

			logger.Info("Starting Metal Operations Server", "version", cmd.Version, "port", port)

			creds := credentials.NewTLS(&tls.Config{
				InsecureSkipVerify: true, //nolint:gosec
				MinVersion:         tls.VersionTLS12,
			})

			conn, err := grpc.NewClient(metalServer, grpc.WithTransportCredentials(creds))
			if err != nil {
				return err
			}

			client := metal.Client{
				Logger: logger.WithGroup("metal"),
				Metal:  metalpb.NewMetalServiceClient(conn),
				Auth:   authpb.NewAuthServiceClient(conn),
			}

			if err := client.Authorize(username, password); err != nil {
				return err
			}

			reporter := handlers.Reporter{
				Logger: logger.WithGroup("reporter"),
				Client: &client,
			}

			mux := http.NewServeMux()

			mw := handlers.Chain(
				handlers.RecoveryMiddleware(logger),
				handlers.RequestIDMiddleware,
				handlers.LoggingMiddleware(logger),
				handlers.DefaultJSONMiddleware,
				client.RefreshTokenMiddleware,
			)

			mux.Handle("/report/{name}", mw(&reporter))

			if err := http.ListenAndServe(fmt.Sprintf(":%d", port), mux); err != nil {
				return err
			}

			return nil
		},
	}

	logging.DefaultJSON = true
	logOpts = logging.NewOptions(cmd.Flags())

	cmd.Flags().IntVar(&port, "port", ops.DefaultPort, "port to listen on")
	cmd.Flags().StringVar(&username, "username", "admin", "username for authentication")
	cmd.Flags().StringVar(&password, "password", "admin", "password for authentication")
	cmd.Flags().StringVar(&metalServer, "metal-server", "localhost:"+strconv.Itoa(metal.DefaultPort),
		"address of the metal server")

	return &cmd
}
