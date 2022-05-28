package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"go.seankhliao.com/webstyle"
	"golang.org/x/exp/slices"
)

func main() {
	var in, out, gtm, baseUrl string
	flag.StringVar(&in, "src", "src", "source directory/file")
	flag.StringVar(&out, "dst", "dst", "destination directory/file")
	flag.StringVar(&gtm, "gtm", "", "google tag manager id")
	flag.StringVar(&baseUrl, "base-url", "", "base url, ex: https://seankhliao.com")
	flag.Parse()

	fi, err := os.Stat(in)
	if err != nil {
		log.Fatalln("stat src", in, err)
	}
	if !fi.IsDir() {
		err := renderSingle(in, out)
		if err != nil {
			log.Fatalln(err)
		}
	} else {
		err := renderMulti(in, out, gtm, baseUrl)
		if err != nil {
			log.Fatalln(err)
		}
	}
}

func renderSingle(in, out string) error {
	inFile, err := os.Open(in)
	if err != nil {
		return fmt.Errorf("open src=%v: %w", in, err)
	}
	defer inFile.Close()
	outFile, err := os.Create(out)
	if err != nil {
		return fmt.Errorf("open dst=%v: %w", out, err)
	}
	render := webstyle.NewRenderer(webstyle.TemplateCompact)
	err = render.Render(outFile, inFile, webstyle.Data{})
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}
	return nil
}

func renderMulti(in, out, gtm, baseUrl string) error {
	render := webstyle.NewRenderer(webstyle.TemplateFull)
	var siteMapTxt bytes.Buffer
	err := filepath.WalkDir(in, func(inPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(in, inPath)
		if err != nil {
			return err
		}
		outPath := filepath.Join(out, relPath)
		if filepath.Ext(inPath) == ".md" {
			outPath = outPath[:len(outPath)-3] + ".html"
		}
		if d.IsDir() {
			return os.MkdirAll(outPath, 0o755)
		}

		inFile, err := os.Open(inPath)
		if err != nil {
			return fmt.Errorf("open src=%v: %w", inPath, err)
		}
		defer inFile.Close()
		outFile, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("open dst=%v: %w", outPath, err)
		}
		defer outFile.Close()

		var main string
		if strings.HasSuffix(relPath, "/index.md") { // exclude root index
			dir := filepath.Dir(inPath)
			des, err := os.ReadDir(dir)
			if err != nil {
				return fmt.Errorf("read dir=%v: %w", dir, err)
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
			main = buf.String()
		}
		if strings.HasSuffix(inPath, ".md") {
			err = render.Render(outFile, inFile, webstyle.Data{
				GTM:  gtm,
				Main: main,
			})
			if err != nil {
				return fmt.Errorf("render src=%v: %w", inPath, err)
			}

			siteMapTxt.WriteString(baseUrl + canonicalPathFromRelPath(relPath) + "\n")
		} else {
			_, err = io.Copy(outFile, inFile)
			if err != nil {
				return fmt.Errorf("copy src=%v: %w", inPath, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk src=%v: %w", in, err)
	}

	err = os.WriteFile(filepath.Join(in, "sitemap.txt"), siteMapTxt.Bytes(), 0o644)
	if err != nil {
		return fmt.Errorf("write sitemap.txt: %w", err)
	}

	return nil
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
