package html

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestTemplates_Reload(t *testing.T) {
	fs := fstest.MapFS{
		"templates/public.html":       {Data: []byte("page")},
		"templates/404.html":          {Data: []byte("not found")},
		"templates/common/style.html": {Data: []byte("style")},
	}

	templates := &Templates{FS: fs}
	require.NoError(t, templates.Run(context.Background()))

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if err := templates.loadTemplates(); err != nil {
					t.Error(err)
				}

				var buf bytes.Buffer
				if err := templates.Execute(&buf, "public.html", nil); err != nil {
					t.Error(err)
				}
				if buf.String() != "page" {
					t.Errorf("unexpected page: %q", buf.String())
				}
			}
		}()
	}
	wg.Wait()

	fs["templates/public.html"].Data = []byte("{{")
	require.Error(t, templates.loadTemplates())

	var buf bytes.Buffer
	require.NoError(t, templates.Execute(&buf, "public.html", nil))
	require.Equal(t, "page", buf.String())
}
