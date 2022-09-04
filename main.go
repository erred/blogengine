package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/iand/logfmtr"
	"go.seankhliao.com/webstyle"
	"golang.org/x/term"
)

func main() {
	log := logfmtr.NewWithOptions(logfmtr.Options{
		Writer:   os.Stderr,
		Colorize: term.IsTerminal(int(os.Stderr.Fd())),
	})
	ctx := context.Background()
	ctx = logr.NewContext(ctx, log)

	err := run(ctx, os.Args)
	if err != nil {
		log.Error(err, "run")
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	log := logr.FromContextOrDiscard(ctx)
	conf, err := newConfig(ctx, args)
	if err != nil {
		return err
	}

	var render webstyle.Renderer
	log.V(1).Info("setting renderer style", "style", conf.Render.Style)
	switch conf.Render.Style {
	case "compact":
		render = webstyle.NewRenderer(webstyle.TemplateCompact)
	case "full":
		render = webstyle.NewRenderer(webstyle.TemplateFull)
	default:
		return fmt.Errorf("unknown renderer style: %s", conf.Render.Style)
	}

	fi, err := os.Stat(conf.Render.Source)
	if err != nil {
		log.Error(err, "stat source", "src", conf.Render.Source)
		return err
	}
	var rendered map[string]*bytes.Buffer
	if !fi.IsDir() {
		rendered, err = renderSingle(ctx, render, conf.Render.Source)
	} else {
		rendered, err = renderMulti(ctx, render, conf.Render.Source, conf.Render.GTM, conf.Render.BaseURL)
	}
	if err != nil {
		log.Error(err, "render source", "src", conf.Render.Source)
		return err
	}

	if conf.Render.Destination != "" {
		err = writeRendered(ctx, conf.Render.Destination, rendered)
		if err != nil {
			log.Error(err, "write rendered result", "dst", conf.Render.Destination)
			return err
		}
	}
	if conf.Firebase.SiteID != "" {
		err = uploadFirebase(ctx, conf.Firebase, rendered)
		if err != nil {
			log.Error(err, "upload to firebase")
			return err
		}
	}

	return nil
}
