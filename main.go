package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/exelban/EndPoll/api"
	"github.com/exelban/EndPoll/internal/html"
	"github.com/exelban/EndPoll/internal/monitor"
	"github.com/exelban/EndPoll/store"
	"github.com/exelban/EndPoll/types"
	"github.com/jessevdk/go-flags"
	"github.com/pkgz/logg"
)

type arguments struct {
	ConfigPath string `long:"config-path" env:"CONFIG_PATH" default:"./config.yaml" description:"path to the configuration file"`

	Storage struct {
		Type string `long:"type" env:"TYPE" default:"bolt" description:"storage type (bolt, sqlite, memory)"`
		Path string `long:"path" env:"PATH" default:"./data" description:"storage path"`
	} `group:"storage" namespace:"storage" env-namespace:"STORAGE"`

	Port  int  `long:"port" env:"PORT" default:"8822" description:"service rest port"`
	Debug bool `long:"debug" env:"DEBUG" description:"debug mode"`
}

type app struct {
	srv *api.Server
	api *api.Rest

	config *types.Cfg
	store  store.Interface
}

//go:embed templates/*
var fs embed.FS
var version = "dev"

func main() {
	var args arguments
	p := flags.NewParser(&args, flags.Default)
	if _, err := p.Parse(); err != nil {
		if flags.WroteHelp(err) {
			os.Exit(0)
		}
		fmt.Printf("error parse args: %v", err)
		os.Exit(1)
	}
	fmt.Println(version)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		log.Print("[INFO] interrupt signal")
		cancel()
	}()

	logg.NewGlobal(os.Stdout)
	if args.Debug {
		logg.DebugMode()
	}

	app, err := create(ctx, args)
	if err != nil {
		log.Printf("[ERROR] create app: %v", err)
		os.Exit(1)
	}

	if err := app.run(ctx); err != nil {
		log.Printf("[ERROR] run app: %v", err)
		os.Exit(1)
	}
}

func create(ctx context.Context, args arguments) (*app, error) {
	cfg, err := types.NewConfig(ctx, args.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("new config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	storage, err := store.New(ctx, args.Storage.Type, args.Storage.Path)
	if err != nil {
		return nil, fmt.Errorf("new store: %w", err)
	}

	return &app{
		srv: &api.Server{
			Port: args.Port,
		},

		api: &api.Rest{
			Monitor: &monitor.Monitor{
				Store:    storage,
				CacheTTL: 5 * time.Second,
			},
			Templates: &html.Templates{
				FS:    fs,
				Debug: args.Debug,
			},
			Version: version,
			Config:  cfg,
		},
		config: cfg,
		store:  storage,
	}, nil
}

func (a *app) run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		a.shutdown()
	}()

	if err := a.api.Templates.Run(ctx); err != nil {
		return fmt.Errorf("generate templates: %w", err)
	}
	if err := a.api.Monitor.Run(ctx, a.config); err != nil {
		return fmt.Errorf("run monitor: %w", err)
	}

	srvErr := make(chan error, 1)
	go func() {
		srvErr <- a.srv.Run(a.api.Router())
	}()

	for {
		select {
		case <-a.config.FW:
			if err := a.config.Reload(); err != nil {
				log.Printf("[ERROR] reload config: %v", err)
				continue
			}
			if err := a.api.Monitor.Run(ctx, a.config); err != nil {
				log.Printf("[ERROR] run monitor: %v", err)
			}
		case err := <-srvErr:
			if err != nil {
				return fmt.Errorf("rest server: %w", err)
			}
			return nil
		case <-ctx.Done():
			log.Print("[DEBUG] terminating...")
			return nil
		}
	}
}

func (a *app) shutdown() {
	if err := a.srv.Shutdown(); err != nil {
		log.Printf("[ERROR] rest shutdown %v", err)
	}
	a.api.Monitor.Stop()
	if err := a.store.Close(); err != nil {
		log.Printf("[ERROR] store close %v", err)
	}
}
