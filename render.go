package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"go.seankhliao.com/webstyle"
	"golang.org/x/exp/slices"
)

const (
	singleKey = ":single"
)

func renderSingle(ctx context.Context, render webstyle.Renderer, in string) (map[string]*bytes.Buffer, error) {
	log := logr.FromContextOrDiscard(ctx).WithValues("src", in)
	log.V(1).Info("rendering single file")
	inFile, err := os.Open(in)
	if err != nil {
		log.Error(err, "open source")
		return nil, err
	}
	defer inFile.Close()
	var buf bytes.Buffer
	err = render.Render(&buf, inFile, webstyle.Data{})
	if err != nil {
		log.Error(err, "render")
		return nil, err
	}
	return map[string]*bytes.Buffer{singleKey: &buf}, nil
}

func renderMulti(ctx context.Context, render webstyle.Renderer, in, gtm, baseUrl string) (map[string]*bytes.Buffer, error) {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("rendering directory", "src", in)

	var siteMapTxt bytes.Buffer
	rendered := make(map[string]*bytes.Buffer)
	fsys := os.DirFS(in)
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		log := log.WithValues("src", p)

		inFile, err := fsys.Open(p)
		if err != nil {
			log.Error(err, "open file")
			return err
		}
		defer inFile.Close()

		var buf bytes.Buffer
		if strings.HasSuffix(p, ".md") {
			data := webstyle.Data{
				GTM: gtm,
			}

			if p == "index.md" { // root index
				data.Desc = `hi, i'm sean, available for adoption by extroverts for the low, low cost of your love.`
			} else if strings.HasSuffix(p, "/index.md") { // exclude root index
				data.Main, err = directoryList(logr.NewContext(ctx, log), fsys, p)
				if err != nil {
					return err
				}
			}

			log.V(1).Info("rendering page")
			err = render.Render(&buf, inFile, data)
			if err != nil {
				log.Error(err, "render markdown")
				return err
			}

			fmt.Fprintf(&siteMapTxt, "%s%s\n", baseUrl, canonicalPathFromRelPath(p))
			p = p[:len(p)-3] + ".html"
		} else {
			log.V(1).Info("copying static file")
			_, err = io.Copy(&buf, inFile)
			if err != nil {
				log.Error(err, "copy file")
				return err
			}
		}

		rendered[p] = &buf

		return nil
	})
	if err != nil {
		log.Error(err, "walk", "src", in)
		return nil, err
	}

	rendered["sitemap.txt"] = &siteMapTxt
	return rendered, nil
}

func directoryList(ctx context.Context, fsys fs.FS, p string) (string, error) {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("creating index listing")
	des, err := fs.ReadDir(fsys, filepath.Dir(p))
	if err != nil {
		log.Error(err, "readdir")
		return "", err
	}

	// reverse order
	slices.SortFunc(des, func(a, b fs.DirEntry) bool { return a.Name() > b.Name() })

	var buf bytes.Buffer
	buf.WriteString("<ul>\n")
	for _, de := range des {
		if de.IsDir() || de.Name() == "index.md" {
			continue
		}
		n := de.Name() // 120XX-YY-ZZ-some-title.md
		if strings.HasPrefix(n, "120") && len(n) > 15 && n[11] == '-' {
			fmt.Fprintf(&buf, `<li><time datetime="%s">%s</time> | <a href="%s">%s</a></li>`,
				n[1:11],          // 20XX-YY-ZZ
				n[:11],           // 120XX-YY-ZZ
				n[:len(n)-3]+"/", // 120XX-YY-ZZ-some-title/
				strings.ReplaceAll(n[12:len(n)-3], "-", " "), // some title
			)
		}
	}
	buf.WriteString("</ul>\n")
	return buf.String(), nil
}

func canonicalPathFromRelPath(in string) string {
	in = strings.TrimSuffix(in, ".md")
	in = strings.TrimSuffix(in, ".html")
	in = strings.TrimSuffix(in, "index")
	if in == "" {
		return "/"
	} else if in[len(in)-1] == '/' {
		return "/" + in
	}
	return "/" + in + "/"
}
