package api

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/exelban/EndPoll/pkg/html"
	"github.com/exelban/EndPoll/pkg/monitor"
	"github.com/exelban/EndPoll/types"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	mhtml "github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
	"github.com/tdewolff/minify/v2/svg"
	"github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
)

type Rest struct {
	Monitor   *monitor.Monitor
	Templates *html.Templates
	UI        *types.UI

	Version string

	minify *minify.M
}

func (s *Rest) Router() *http.ServeMux {
	s.minify = minify.New()
	s.minify.AddFunc("text/html", mhtml.Minify)
	s.minify.AddFunc("text/css", css.Minify)
	s.minify.AddFunc("image/svg+xml", svg.Minify)
	s.minify.AddFunc("application/javascript", js.Minify)

	router := NewRouter(Recoverer, CORS, Ping(), Info("EndPoll", s.Version))

	router.HandleFunc("GET /", s.public)
	router.HandleFunc("GET /{id}", s.public)
	router.HandleFunc("GET /static/", s.static)

	router.HandleFunc("GET /response-time/{id}", s.responseTime)

	return router.mux
}

func (s *Rest) public(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	var stats *types.Stats = nil
	var err error
	if id == "" {
		stats, err = s.Monitor.Stats(ctx)
	} else {
		stats, err = s.Monitor.StatsByID(ctx, id, false)
	}
	if err != nil {
		if errors.Is(err, types.ErrHostNotFound) {
			s.notFound(w, r)
			return
		}
		log.Printf("[ERROR] get stats: %v", err)
		http.Error(w, fmt.Sprintf("error get stats: %v", err), http.StatusInternalServerError)
		return
	}

	if stats == nil {
		http.Error(w, "stats not found", http.StatusNotFound)
		return
	}

	data := struct {
		Data     *types.Stats
		Settings *types.UI
	}{
		Data:     stats,
		Settings: s.UI,
	}

	var buf bytes.Buffer
	if err := s.Templates.Public.Execute(&buf, data); err != nil {
		log.Printf("[ERROR] generate public html: %v", err)
		http.Error(w, fmt.Sprintf("error generate public html: %v", err), http.StatusInternalServerError)
		return
	}

	minified, err := s.minify.Bytes("text/html", buf.Bytes())
	if err != nil {
		log.Printf("[ERROR] minify public html: %v", err)
		http.Error(w, fmt.Sprintf("error minify public html: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(minified)
}

func (s *Rest) notFound(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	if err := s.Templates.NotFound.Execute(&buf, nil); err != nil {
		log.Printf("[ERROR] generate public html: %v", err)
		http.Error(w, fmt.Sprintf("error generate not found html: %v", err), http.StatusInternalServerError)
		return
	}

	minified, err := s.minify.Bytes("text/html", buf.Bytes())
	if err != nil {
		log.Printf("[ERROR] minify not found html: %v", err)
		http.Error(w, fmt.Sprintf("error minify not found html: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(minified)
}

func (s *Rest) static(w http.ResponseWriter, r *http.Request) {
	path := fmt.Sprintf("templates%s", r.URL.Path)
	if _, err := s.Templates.FS.Open(path); err != nil {
		s.notFound(w, r)
		return
	}
	http.ServeFileFS(w, r, s.Templates.FS, path)
}

func (s *Rest) responseTime(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	x, y, err := s.Monitor.ResponseTime(ctx, id)
	if err != nil {
		if errors.Is(err, types.ErrHostNotFound) {
			s.notFound(w, r)
			return
		}
		log.Printf("[ERROR] get response time: %v", err)
		http.Error(w, fmt.Sprintf("error get response time: %v", err), http.StatusInternalServerError)
		return
	}

	if len(x) == 1 && len(y) == 1 {
		yesterday := x[0].AddDate(0, 0, -1)
		x = append(x, yesterday)
		y = append(y, 0)
	}

	graph := chart.Chart{
		Height: 320,
		Background: chart.Style{
			FillColor: drawing.Color{R: 0, G: 0, B: 0, A: 1},
		},
		Canvas: chart.Style{
			FillColor: drawing.Color{R: 0, G: 0, B: 0, A: 1},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				XValues: x,
				YValues: y,
			},
		},
	}

	w.Header().Set("Content-Type", "image/png")
	if err := graph.Render(chart.PNG, w); err != nil {
		log.Printf("[ERROR] render chart: %v", err)
		http.Error(w, fmt.Sprintf("error render chart: %v", err), http.StatusInternalServerError)
	}
}
