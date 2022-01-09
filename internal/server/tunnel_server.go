package server

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/hatlonely/go-kit/logger"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
)

type TunnelServerOptions struct {
	TunnelPort   int `flag:"usage: tunnel server listen port" dft:"5080"`
	ServerPort   int `flag:"usage: server listen port" dft:"80"`
	WorkerNum    int `flag:"usage: worker goroutine number" dft:"32"`
	AcceptNum    int `flag:"usage: accept goroutine number" dft:"1"`
	ConnQueueLen int `flag:"usage: connection queue length. if the queue is full, connection will drop immediately" dft:"20"`
}

func NewTunnelServerWithOptions(options *TunnelServerOptions) (*TunnelServer, error) {
	tunnelListener, err := net.Listen("tcp", fmt.Sprintf(":%v", options.TunnelPort))
	if err != nil {
		return nil, errors.Wrap(err, "net.Listen tunnel failed")
	}
	serverListener, err := net.Listen("tcp", fmt.Sprintf(":%v", options.ServerPort))
	if err != nil {
		return nil, errors.Wrap(err, "net.Listen server failed")
	}

	return &TunnelServer{
		options:        options,
		tunnelListener: tunnelListener,
		serverListener: serverListener,
		log:            logger.NewStdoutTextLogger(),
		clientConnChan: make(chan net.Conn, options.ConnQueueLen),
	}, nil
}

type TunnelServer struct {
	options *TunnelServerOptions

	tunnelListener net.Listener
	serverListener net.Listener
	log            *logger.Logger

	wg             sync.WaitGroup
	clientConnChan chan net.Conn

	stop bool
}

func (s *TunnelServer) SetLogger(log *logger.Logger) {
	s.log = log
}

func (s *TunnelServer) Run() {
	for i := 0; i < s.options.AcceptNum; i++ {
		go func(i int) {
			for !s.stop {
				log := s.log.WithFields(map[string]interface{}{
					"acceptNo": i,
					"acceptID": uuid.NewV4().String(),
				})
				log.Info("accept begin")
				if err := s.accept(); err != nil {
					log.Warn(err.Error())
				}
				log.Info("accept end")
			}
		}(i)
	}

	for i := 0; i < s.options.WorkerNum; i++ {
		s.wg.Add(1)
		go func(i int) {
			for clientConn := range s.clientConnChan {
				log := s.log.WithFields(map[string]interface{}{
					"workerNo": i,
					"workerID": uuid.NewV4().String(),
				})
				log.Info("work begin")
				if err := s.work(log, clientConn); err != nil {
					log.Warn(err.Error())
				}
				log.Info("work end")
			}
			s.wg.Done()
		}(i)
	}
}

func (s *TunnelServer) Stop() {
	s.stop = true
	close(s.clientConnChan)
	s.wg.Wait()
	s.serverListener.Close()
	s.tunnelListener.Close()
}

// accept 协程，接受链接，链接队列满或者服务关闭，拒绝链接
func (s *TunnelServer) accept() error {
	clientConn, err := s.serverListener.Accept()
	if err != nil {
		return errors.Wrap(err, "serverListener.Accept failed")
	}
	if len(s.clientConnChan) == s.options.ConnQueueLen {
		clientConn.Close()
		return errors.New("reject cause too many connections")
	}
	if s.stop {
		clientConn.Close()
		return errors.New("reject cause server stop")
	}

	s.clientConnChan <- clientConn
	return nil
}

// work 协程，处理链接，将数据转发到 tunnel
func (s *TunnelServer) work(log *logger.Logger, clientConn net.Conn) error {
	var err error
	defer clientConn.Close()

	var tunnelConn net.Conn
	for {
		tunnelConn, err = s.tunnelListener.Accept()
		if err != nil {
			return errors.Wrap(err, "tunnelListener.Accept failed")
		}
		// 握手
		// server -> agent:  1
		// agent  -> server: 2
		n, err := tunnelConn.Write([]byte{1})
		if err == nil && n == 1 {
			buf := make([]byte, 1)
			n, err = tunnelConn.Read(buf)
			if err == nil && n == 1 && buf[0] == 2 {
				break
			}
		}
		log.Warnf("handshake failed, err: [%v]", err)
		tunnelConn.Close()
	}
	defer tunnelConn.Close()

	tunnelReader := bufio.NewReader(tunnelConn)
	tunnelWriter := bufio.NewWriter(tunnelConn)
	serverReader := bufio.NewReader(clientConn)
	serverWriter := bufio.NewWriter(clientConn)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		defer tunnelConn.Close()
		defer clientConn.Close()
		defer log.Info("client -> tunnel worker quit")
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
		defer clientConn.Close()
		defer log.Info("tunnel -> client worker quit")
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
