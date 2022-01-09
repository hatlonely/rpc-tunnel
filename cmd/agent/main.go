package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hatlonely/rpc-tunnel/internal/agent"

	"github.com/hatlonely/go-kit/bind"
	"github.com/hatlonely/go-kit/flag"
	"github.com/hatlonely/go-kit/logger"
	"github.com/hatlonely/go-kit/refx"
)

var Version string

type AgentOptions struct {
	flag.Options
	Agent                   agent.TunnelAgentOptions
	UseStdoutJsonLogger     bool
	UseRotateFileJsonLogger bool
	UseCustomLogger         bool `flag:"usage: if ture, the logger options will enable"`
	Logger                  logger.Options
}

func main() {
	var options AgentOptions
	refx.Must(flag.Struct(&options, refx.WithCamelName()))
	refx.Must(flag.Parse(flag.WithJsonVal()))
	if options.Help {
		fmt.Println(flag.Usage())
		fmt.Print(`Example:
  tunnel-agent --agent.tunnelAddr 127.0.0.1:5080 --agent.serverAddr 127.0.0.1:9000
  tunnel-agent --agent.tunnelAddr 127.0.0.1:5080 --agent.serverAddr 127.0.0.1:9000 --agent.workerNum 16 --agent.keepAlivePeriod 30s --useStdoutJsonLogger
  tunnel-agent --agent.tunnelAddr 127.0.0.1:5080 --agent.serverAddr 127.0.0.1:9000 --useCustomLogger --logger.level Info --logger.writers '[{
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

	refx.Must(bind.Bind(&options, []bind.Getter{flag.Instance(), bind.NewEnvGetter(bind.WithEnvPrefix("TUNNEL_AGENT"))}, refx.WithCamelName()))

	agent, err := agent.NewTunnelAgentWithOptions(&options.Agent)
	refx.Must(err)
	if options.UseStdoutJsonLogger {
		agent.SetLogger(logger.NewStdoutJsonLogger())
	}
	if options.UseRotateFileJsonLogger {
		log, err := logger.NewLoggerWithOptions(&logger.Options{
			Level: "Info",
			Writers: []refx.TypeOptions{{
				Type: "RotateFile",
				Options: &logger.RotateFileWriterOptions{
					Level:    "Info",
					Filename: "log/tunnel-agent.log",
					MaxAge:   24 * time.Hour,
					Formatter: logger.FormatterOptions{
						Type: "Json",
					},
				},
			}},
		})
		refx.Must(err)
		agent.SetLogger(log)
	}
	if options.UseCustomLogger {
		log, err := logger.NewLoggerWithOptions(&options.Logger, refx.WithCamelName())
		refx.Must(err)
		agent.SetLogger(log)
	}

	agent.Run()
	defer agent.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}
