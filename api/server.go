package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// Server - rest server struct
type Server struct {
	Address string
	Port    int

	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration

	srv *http.Server
	mu  sync.Mutex
}

// Run - will initialize server and run it on provided port
func (s *Server) Run(router http.Handler) error {
	s.mu.Lock()
	if s.Address == "*" {
		s.Address = ""
	}
	if s.Port == 0 {
		s.Port = 8080
	}

	if s.ReadHeaderTimeout == 0 {
		s.ReadHeaderTimeout = 10 * time.Second
	}
	if s.WriteTimeout == 0 {
		s.WriteTimeout = 30 * time.Second
	}
	if s.IdleTimeout == 0 {
		s.IdleTimeout = 60 * time.Second
	}

	addr := "http://localhost"
	if s.Address != "" {
		addr = s.Address
	}
	log.Printf("[INFO] http rest server on %s:%d", addr, s.Port)

	s.srv = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", s.Address, s.Port),
		Handler:           router,
		ReadHeaderTimeout: s.ReadHeaderTimeout,
		WriteTimeout:      s.WriteTimeout,
		IdleTimeout:       s.IdleTimeout,
	}
	srv := s.srv
	s.mu.Unlock()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("start http server, %s", err)
	}

	return nil
}

// Shutdown - shutdown rest server
func (s *Server) Shutdown() error {
	log.Print("[INFO] shutdown rest server")

	s.mu.Lock()
	srv := s.srv
	s.mu.Unlock()
	if srv == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	return srv.Shutdown(ctx)
}
