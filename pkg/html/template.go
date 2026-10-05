package html

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"
)

type Templates struct {
	FS    fs.FS
	Debug bool

	Public   *template.Template
	NotFound *template.Template
}

func (t *Templates) Run(ctx context.Context) error {
	if err := t.loadTemplates(); err != nil {
		return fmt.Errorf("load templates: %w", err)
	}

	if t.Public == nil || t.NotFound == nil {
		return fmt.Errorf("templates not loaded")
	}
	if !t.Debug {
		return nil
	}

	if _, err := os.Stat("templates"); err != nil {
		log.Printf("[DEBUG] local templates directory not found, live reload disabled")
		return nil
	}

	changeLog := make(map[string]chan bool)
	if err := filepath.Walk("templates", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}
		ch, err := watchForFile(ctx, path)
		if err != nil {
			return fmt.Errorf("watch for file %s: %w", path, err)
		}
		changeLog[path] = ch
		return nil
	}); err != nil {
		return fmt.Errorf("walk: %w", err)
	}

	for path, ch := range changeLog {
		go func(path string, ch chan bool) {
		loop:
			for {
				select {
				case <-ch:
					if err := t.loadTemplates(); err != nil {
						log.Printf("[ERROR] load templates: %v", err)
					} else {
						log.Printf("[DEBUG] reloaded %s", path)
					}
				case <-ctx.Done():
					close(ch)
					log.Printf("[DEBUG] watch for %s stopped", path)
					break loop
				}
			}
		}(path, ch)
	}

	return nil
}

func (t *Templates) loadTemplates() error {
	filesystem := t.FS
	localFS := os.DirFS(".")
	if t.Debug {
		if _, err := fs.Stat(localFS, "templates/common/public.html"); err == nil {
			filesystem = localFS
		}
	}

	templ, err := template.ParseFS(filesystem, "templates/common/*.html", "templates/*.html")
	if err != nil {
		return fmt.Errorf("parse files: %w", err)
	}

	t.Public = templ.Lookup("public.html")
	t.NotFound = templ.Lookup("404.html")

	return nil
}

func watchForFile(ctx context.Context, path string) (chan bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("file not found %s: %v", path, err)
	}
	modTimestamp := fi.ModTime()
	ch := make(chan bool)

	go func() {
		tk := time.NewTicker(time.Second)
		for {
			select {
			case <-tk.C:
				fi, err = os.Stat(path)
				if err != nil {
					continue
				}
				if fi.ModTime() != modTimestamp {
					modTimestamp = fi.ModTime()
					ch <- true
				}
			case <-ctx.Done():
				tk.Stop()
				return
			}
		}
	}()

	return ch, nil
}
