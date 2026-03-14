package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/exelban/EndPoll/api"
	"github.com/exelban/EndPoll/pkg/html"
	"github.com/exelban/EndPoll/pkg/monitor"
	"github.com/exelban/EndPoll/store"
	"github.com/exelban/EndPoll/types"
	"github.com/jessevdk/go-flags"
	"github.com/pkgz/logg"
)

type arguments struct {
	ConfigPath string `long:"config-path" env:"CONFIG_PATH" default:"./config.yaml" description:"path to the configuration file"`

	Storage struct {
		Type string `long:"type" env:"TYPE" default:"bolt" description:"storage type"`
		Path string `long:"path" env:"PATH" default:"./data" description:"storage path"`
	} `group:"storage" namespace:"storage" env-namespace:"STORAGE"`

	SMTP struct {
		Host     string   `long:"host" env:"HOST" description:"SMTP host"`
		Port     int      `long:"port" env:"PORT" default:"25" description:"SMTP port"`
		Username string   `long:"username" env:"USERNAME" description:"SMTP user"`
		Password string   `long:"password" env:"PASSWORD" description:"SMTP password"`
		From     string   `long:"from" env:"FROM" description:"SMTP sender email"`
		To       []string `long:"to" env:"TO" description:"SMTP receiver email"`
	} `group:"smtp" namespace:"smtp" env-namespace:"SMTP"`

	Port  int  `long:"port" env:"PORT" default:"8822" description:"service rest port"`
	Debug bool `long:"debug" env:"DEBUG" description:"debug mode"`
}

type app struct {
	srv *api.Server
	api *api.Rest

	config *types.Cfg
	store  store.Interface

	args arguments
}

//go:embed templates/*
var fs embed.FS
var version = "dev"

func main() {
	fmt.Println(version)

	var args arguments
	p := flags.NewParser(&args, flags.Default)
	if _, err := p.Parse(); err != nil {
		fmt.Printf("error parse args: %v", err)
		os.Exit(1)
	}

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

	storage, err := store.New(ctx, args.Storage.Type, args.Storage.Path, cfg)
	if err != nil {
		return nil, fmt.Errorf("new store: %w", err)
	}

	return &app{
		srv: &api.Server{
			Port: args.Port,
		},

		api: &api.Rest{
			Monitor: &monitor.Monitor{
				Store: storage,
			},
			Templates: &html.Templates{
				FS:    fs,
				Debug: args.Debug,
			},
			Version: version,
			UI:      &cfg.UI,
		},
		config: cfg,
		store:  storage,

		args: args,
	}, nil
}

func (a *app) run(ctx context.Context) error {
	if err := a.api.Templates.Run(ctx); err != nil {
		log.Printf("[ERROR] generate templates: %v", err)
	}

	go func() {
		if err := a.srv.Run(a.api.Router()); err != nil {
			log.Printf("[ERROR] run rest server: %v", err)
		}
	}()

	for {
		select {
		case <-a.config.FW:
			if err := a.config.Parse(); err != nil {
				log.Printf("[ERROR] parse config: %v", err)
			}
			if err := a.config.Validate(); err != nil {
				log.Printf("[ERROR] validate config: %v", err)
			}
			if err := a.api.Monitor.Run(a.config); err != nil {
				log.Printf("[ERROR] run monitor: %v", err)
			}
		case <-ctx.Done():
			log.Print("[DEBUG] terminating...")

			if err := a.srv.Shutdown(); err != nil {
				log.Printf("[ERROR] rest shutdown %v", err)
			}
			if err := a.store.Close(); err != nil {
				log.Printf("[ERROR] store close %v", err)
			}

			log.Print("[INFO] terminated")
			return nil
		}
	}
}
