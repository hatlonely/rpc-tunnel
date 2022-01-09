package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hatlonely/rpc-tunnel/internal/server"

	"github.com/hatlonely/go-kit/bind"
	"github.com/hatlonely/go-kit/flag"
	"github.com/hatlonely/go-kit/logger"
	"github.com/hatlonely/go-kit/refx"
)

var Version string

type ServerOptions struct {
	flag.Options
	Server                  server.TunnelServerOptions
	UseStdoutJsonLogger     bool
	UseRotateFileJsonLogger bool
	UseCustomLogger         bool `flag:"usage: if ture, the logger options will enable"`
	Logger                  logger.Options
}

func main() {
	var options ServerOptions
	refx.Must(flag.Struct(&options, refx.WithCamelName()))
	refx.Must(flag.Parse(flag.WithJsonVal()))
	if options.Help {
		fmt.Println(flag.Usage())
		fmt.Print(`Example:
  tunnel-server --server.serverPort 8000
  tunnel-server --server.serverPort 8000 --server.tunnelPort 5080 --server.workerNum 16 --server.acceptNum 5 --server.connQueueLen 200 --useStdoutJsonLogger
  tunnel-server --server.serverPort 8000 --useCustomLogger --logger.level Info --logger.writers '[{
        "type": "RotateFile",
        "options": {
          "level": "Info",
          "filename": "log/test.log",
          "maxAge": "24h",
          "formatter": {
            "type": "Json"
          }
        }
      }]'
`)
		return

	}
	if options.Version {
		fmt.Println(Version)
		return
	}

	refx.Must(bind.Bind(&options, []bind.Getter{flag.Instance(), bind.NewEnvGetter(bind.WithEnvPrefix("TUNNEL_SERVER"))}, refx.WithCamelName()))

	server, err := server.NewTunnelServerWithOptions(&options.Server)
	refx.Must(err)
	if options.UseStdoutJsonLogger {
		server.SetLogger(logger.NewStdoutJsonLogger())
	}
	if options.UseRotateFileJsonLogger {
		log, err := logger.NewLoggerWithOptions(&logger.Options{
			Level: "Info",
			Writers: []refx.TypeOptions{{
				Type: "RotateFile",
				Options: &logger.RotateFileWriterOptions{
					Level:    "Info",
					Filename: "log/tunnel-server.log",
					MaxAge:   24 * time.Hour,
					Formatter: logger.FormatterOptions{
						Type: "Json",
					},
				},
			}},
		})
		refx.Must(err)
		server.SetLogger(log)
	}
	if options.UseCustomLogger {
		log, err := logger.NewLoggerWithOptions(&options.Logger, refx.WithCamelName())
		refx.Must(err)
		server.SetLogger(log)
	}

	server.Run()
	defer server.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}
