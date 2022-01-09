package agent

import (
	"bufio"
	"io"
	"net"
	"sync"
	"time"

	"github.com/hatlonely/go-kit/logger"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
)

type TunnelAgentOptions struct {
	TunnelAddr      string        `flag:"usage: tunnel server address" dft:"127.0.0.1:5080"`
	ServerAddr      string        `flag:"usage: server address"`
	WorkerNum       int           `flag:"usage: worker goroutine number" dft:"32"`
	KeepAlivePeriod time.Duration `flag:"usage: tunnel connection keep alive period" dft:"20s"`
}

func NewTunnelAgentWithOptions(options *TunnelAgentOptions) (*TunnelAgent, error) {
	return &TunnelAgent{
		options: options,
		log:     logger.NewStdoutTextLogger(),
	}, nil
}

type TunnelAgent struct {
	options *TunnelAgentOptions
	log     *logger.Logger

	wg sync.WaitGroup

	stop bool
}

func (a *TunnelAgent) SetLogger(log *logger.Logger) {
	a.log = log
}

func (a *TunnelAgent) Run() {
	for i := 0; i < a.options.WorkerNum; i++ {
		a.wg.Add(1)
		go func(i int) {
			for !a.stop {
				log := a.log.WithFields(map[string]interface{}{
					"workerNo": i,
					"workerID": uuid.NewV4().String(),
				})
				log.Info("work begin")
				if err := a.work(log); err != nil {
					log.Warn(err.Error())
				}
				log.Info("work end")
			}
			a.wg.Done()
		}(i)
	}
}

func (a *TunnelAgent) Stop() {
	a.stop = true
	a.wg.Wait()
}

func (a *TunnelAgent) work(log *logger.Logger) error {
	var tunnelConn net.Conn
	var err error
	for {
		tunnelConn, err = net.Dial("tcp", a.options.TunnelAddr)
		if err != nil {
			return errors.Wrapf(err, "net.Dial tunnel [%s] failed", a.options.TunnelAddr)
		}
		// 握手
		// server -> agent:  1
		// agent  -> server: 2
		buf := make([]byte, 1)
		n, err := tunnelConn.Read(buf)
		if err == nil && n == 1 && buf[0] == 1 {
			n, err = tunnelConn.Write([]byte{2})
			if err == nil && n == 1 {
				break
			}
		}
		log.Warnf("handshake failed, err: [%v]", err)
		tunnelConn.Close()
	}
	defer tunnelConn.Close()
	serverConn, err := net.Dial("tcp", a.options.ServerAddr)
	if err != nil {
		return errors.Wrapf(err, "net.Dial server [%s] failed", a.options.ServerAddr)
	}
	defer serverConn.Close()
	if err := tunnelConn.(*net.TCPConn).SetKeepAlive(true); err != nil {
		return errors.Wrapf(err, "tunnelConn.SetKeepAlive failed")
	}
	if err := tunnelConn.(*net.TCPConn).SetKeepAlivePeriod(a.options.KeepAlivePeriod); err != nil {
		return errors.Wrapf(err, "tunnelConn.SetKeepAlivePeriod failed")
	}

	tunnelReader := bufio.NewReader(tunnelConn)
	tunnelWriter := bufio.NewWriter(tunnelConn)
	serverReader := bufio.NewReader(serverConn)
	serverWriter := bufio.NewWriter(serverConn)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		defer tunnelConn.Close()
		defer serverConn.Close()
		defer log.Info("server -> tunnel worker quit")
		buf := make([]byte, 1024)
		for {
			n, err := serverReader.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Warn(err.Error())
				}
				return
			}
			if _, err := tunnelWriter.Write(buf[:n]); err != nil {
				log.Warn(err.Error())
				return
			}
			if err := tunnelWriter.Flush(); err != nil {
				log.Warn(err.Error())
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		defer tunnelConn.Close()
		defer serverConn.Close()
		defer log.Info("tunnel -> server worker quit")
		buf := make([]byte, 1024)
		for {
			n, err := tunnelReader.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Warn(err.Error())
				}
				return
			}
			if _, err := serverWriter.Write(buf[:n]); err != nil {
				log.Warn(err.Error())
				return
			}
			if err := serverWriter.Flush(); err != nil {
				log.Warn(err.Error())
				return
			}
		}
	}()
	wg.Wait()

	return nil
}
