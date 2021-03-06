package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/qdm12/golibs/logging"
)

type Server interface {
	Run(ctx context.Context, wg *sync.WaitGroup)
}

type server struct {
	address        string
	logger         logging.Logger
	restartOpenvpn chan<- struct{}
	restartUnbound chan<- struct{}
}

func New(address string, logger logging.Logger, restartOpenvpn, restartUnbound chan<- struct{}) Server {
	return &server{
		address:        address,
		logger:         logger.WithPrefix("http server: "),
		restartOpenvpn: restartOpenvpn,
		restartUnbound: restartUnbound,
	}
}

func (s *server) Run(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	server := http.Server{Addr: s.address, Handler: s.makeHandler()}
	go func() {
		defer wg.Done()
		<-ctx.Done()
		s.logger.Warn("context canceled: exiting loop")
		defer s.logger.Warn("loop exited")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("failed shutting down: %s", err)
		}
	}()
	s.logger.Info("listening on %s", s.address)
	err := server.ListenAndServe()
	if err != nil && ctx.Err() != context.Canceled {
		s.logger.Error(err)
	}
}

func (s *server) makeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.logger.Info("HTTP %s %s", r.Method, r.RequestURI)
		switch r.Method {
		case http.MethodGet:
			switch r.RequestURI {
			case "/openvpn/actions/restart":
				s.restartOpenvpn <- struct{}{}
			case "/unbound/actions/restart":
				s.restartUnbound <- struct{}{}
			default:
				routeDoesNotExist(s.logger, w, r)
			}
		default:
			routeDoesNotExist(s.logger, w, r)
		}
	}
}

func routeDoesNotExist(logger logging.Logger, w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusBadRequest)
	_, err := w.Write([]byte(fmt.Sprintf("Nothing here for %s %s", r.Method, r.RequestURI)))
	if err != nil {
		logger.Error(err)
	}
}
