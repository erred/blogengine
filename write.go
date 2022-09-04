package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
)

func writeRendered(ctx context.Context, out string, rendered map[string]*bytes.Buffer) error {
	log := logr.FromContextOrDiscard(ctx).WithValues("dst", out)

	for p, buf := range rendered {
		if p == singleKey {
			p = out
		} else {
			p = filepath.Join(out, p)
		}

		dir := filepath.Dir(p)
		err := os.MkdirAll(dir, 0o755)
		if err != nil {
			log.Error(err, "create parent directories", "dir", dir)
			return err
		}
		log.V(1).Info("write file", "file", "p")
		err = os.WriteFile(p, buf.Bytes(), 0o644)
		if err != nil {
			log.Error(err, "write file")
			return err
		}
	}
	return nil
}
