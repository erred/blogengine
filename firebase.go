package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-logr/logr"
	"golang.org/x/oauth2/google"
	firebasehosting "google.golang.org/api/firebasehosting/v1beta1"
)

func uploadFirebase(ctx context.Context, conf ConfigFirebase, rendered map[string]*bytes.Buffer) error {
	log := logr.FromContextOrDiscard(ctx)

	pathToHash := make(map[string]string)
	hashToGzip := make(map[string]io.Reader)
	for p, buf := range rendered {
		zipped := new(bytes.Buffer)
		summed := sha256.New()
		gzw := gzip.NewWriter(io.MultiWriter(zipped, summed))
		_, err := io.Copy(gzw, buf)
		if err != nil {
			log.Error(err, "zip file", "file", p)
		}
		gzw.Close()
		sum := hex.EncodeToString(summed.Sum(nil))

		pathToHash["/"+p] = sum
		hashToGzip[sum] = zipped
	}

	httpClient, err := google.DefaultClient(ctx, "https://www.googleapis.com/auth/cloud-platform", "https://www.googleapis.com/auth/firebase")
	if err != nil {
		return fmt.Errorf("create http client: %w", err)
	}

	client, err := firebasehosting.NewService(ctx)
	if err != nil {
		return fmt.Errorf("create firebase client: %w", err)
	}

	site, version, err := createVersion(ctx, client, conf)
	if err != nil {
		return err
	}

	toUpload, uploadURL, err := getRequiredUploads(ctx, client, version, pathToHash)
	if err != nil {
		return err
	}

	err = uploadFiles(ctx, client, httpClient, version, toUpload, uploadURL, hashToGzip)
	if err != nil {
		return err
	}

	err = release(ctx, client, site, version)
	if err != nil {
		return err
	}

	return nil
}

func createVersion(ctx context.Context, client *firebasehosting.Service, conf ConfigFirebase) (string, string, error) {
	log := logr.FromContextOrDiscard(ctx)

	servingConf := &firebasehosting.ServingConfig{
		CleanUrls:             true,
		TrailingSlashBehavior: "ADD",
	}
	for _, header := range conf.Headers {
		servingConf.Headers = append(servingConf.Headers, &firebasehosting.Header{
			Glob:    header.Glob,
			Headers: header.Headers,
		})
	}
	for _, redirect := range conf.Redirects {
		servingConf.Redirects = append(servingConf.Redirects, &firebasehosting.Redirect{
			Glob:       redirect.Glob,
			Location:   redirect.Location,
			StatusCode: int64(redirect.StatusCode),
		})
	}

	siteID := "sites/" + conf.SiteID
	version, err := client.Sites.Versions.Create(siteID, &firebasehosting.Version{
		Config: servingConf,
	}).Context(ctx).Do()
	if err != nil {
		log.Error(err, "create version", "site", siteID)
		return "", "", err
	}

	log.V(1).Info("created version", "version", version.Name)
	return siteID, version.Name, nil
}

func getRequiredUploads(ctx context.Context, client *firebasehosting.Service, version string, pathToHash map[string]string) ([]string, string, error) {
	log := logr.FromContextOrDiscard(ctx)

	populateResponse, err := client.Sites.Versions.PopulateFiles(version, &firebasehosting.PopulateVersionFilesRequest{
		Files: pathToHash,
	}).Context(ctx).Do()
	if err != nil {
		log.Error(err, "populate files", "version", version)
		return nil, "", err
	}

	log.V(1).Info("got requied uploads", "to_upload", len(populateResponse.UploadRequiredHashes))
	return populateResponse.UploadRequiredHashes, populateResponse.UploadUrl, nil
}

func uploadFiles(ctx context.Context, client *firebasehosting.Service, httpClient *http.Client, version string, toUpload []string, uploadURL string, hashToGzip map[string]io.Reader) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("uploading required files", "to_upload", len(toUpload), "total", len(hashToGzip))
	for _, uploadHash := range toUpload {
		endpoint := uploadURL + "/" + uploadHash
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, hashToGzip[uploadHash])
		if err != nil {
			log.Error(err, "create request")
			return err
		}
		req.Header.Set("content-type", "application/octet-stream")
		res, err := httpClient.Do(req)
		if err != nil {
			log.Error(err, "upload", "hash", uploadHash)
			return err
		}
		if res.StatusCode != 200 {
			log.Error(err, "non 200 status", "hash", uploadHash, "status", res.Status)
			return errors.New(res.Status)
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}

	patchResponse, err := client.Sites.Versions.Patch(version, &firebasehosting.Version{
		Status: "FINALIZED",
	}).Context(ctx).Do()
	if err != nil {
		log.Error(err, "patch to finalize", "version", version)
		return err
	}
	if patchResponse.Status != "FINALIZED" {
		log.Error(errors.New(patchResponse.Status), "status not finalized")
		return errors.New(patchResponse.Status)
	}

	log.V(1).Info("finalized version", "version", version)
	return nil
}

func release(ctx context.Context, client *firebasehosting.Service, site, version string) error {
	log := logr.FromContextOrDiscard(ctx)

	_, err := client.Sites.Releases.Create(site, &firebasehosting.Release{}).VersionName(version).Context(ctx).Do()
	if err != nil {
		log.Error(err, "release", "version", version)
		return err
	}

	log.V(1).Info("released", "version", version)
	return nil
}
