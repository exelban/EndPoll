package html

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/exelban/EndPoll/pkg"
)

type Templates struct {
	FS    fs.FS
	Debug bool

	templates *template.Template
	mu        sync.RWMutex
}

func (t *Templates) Run(ctx context.Context) error {
	if err := t.loadTemplates(); err != nil {
		return fmt.Errorf("load templates: %w", err)
	}

	if !t.Debug {
		return nil
	}

	if _, err := os.Stat("templates"); err != nil {
		log.Printf("[DEBUG] local templates directory not found, live reload disabled")
		return nil
	}

	changeLog := make(map[string]<-chan time.Time)
	if err := filepath.Walk("templates", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}
		ch, err := pkg.WatchFile(ctx, path, time.Second, false)
		if err != nil {
			return fmt.Errorf("watch for file %s: %w", path, err)
		}
		changeLog[path] = ch
		return nil
	}); err != nil {
		return fmt.Errorf("walk: %w", err)
	}

	for path, ch := range changeLog {
		go func(path string, ch <-chan time.Time) {
		loop:
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						return
					}
					if err := t.loadTemplates(); err != nil {
						log.Printf("[ERROR] load templates: %v", err)
					} else {
						log.Printf("[DEBUG] reloaded %s", path)
					}
				case <-ctx.Done():
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
		if _, err := fs.Stat(localFS, "templates/public.html"); err == nil {
			filesystem = localFS
		}
	}

	templ, err := template.ParseFS(filesystem, "templates/common/*.html", "templates/*.html")
	if err != nil {
		return fmt.Errorf("parse files: %w", err)
	}

	if templ.Lookup("public.html") == nil || templ.Lookup("404.html") == nil {
		return fmt.Errorf("templates not loaded")
	}

	t.mu.Lock()
	t.templates = templ
	t.mu.Unlock()

	return nil
}

func (t *Templates) Execute(w io.Writer, name string, data any) error {
	t.mu.RLock()
	templ := t.templates
	t.mu.RUnlock()

	if templ == nil {
		return fmt.Errorf("templates not loaded")
	}

	return templ.ExecuteTemplate(w, name, data)
}
