// Copyright 2014 go-dockerclient authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/moby/go-archive"
)

func TestBuildImageMultipleContextsError(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	opts := BuildImageOptions{
		Name:                "testImage",
		NoCache:             true,
		CacheFrom:           []string{"a", "b", "c"},
		SuppressOutput:      true,
		RmTmpContainer:      true,
		ForceRmTmpContainer: true,
		InputStream:         &buf,
		OutputStream:        &buf,
		ContextDir:          "testing/data",
	}
	err := client.BuildImage(opts)
	if !errors.Is(err, ErrMultipleContexts) {
		t.Errorf("BuildImage: providing both InputStream and ContextDir should produce an error")
	}
}

func TestBuildImageContextDirDockerignoreParsing(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)

	if err := os.Symlink("doesnotexist", "testing/data/symlink"); err != nil {
		t.Errorf("error creating symlink on demand: %s", err)
	}
	defer func() {
		if err := os.Remove("testing/data/symlink"); err != nil {
			t.Errorf("error removing symlink on demand: %s", err)
		}
	}()
	workingdir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	opts := BuildImageOptions{
		Name:                "testImage",
		NoCache:             true,
		CacheFrom:           []string{"a", "b", "c"},
		SuppressOutput:      true,
		RmTmpContainer:      true,
		ForceRmTmpContainer: true,
		OutputStream:        &buf,
		ContextDir:          filepath.Join(workingdir, "testing", "data"),
	}
	err = client.BuildImage(opts)
	if err != nil {
		t.Fatal(err)
	}
	reqBody := fakeRT.requests[0].Body
	tmpdir, err := unpackBodyTarball(reqBody)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err = os.RemoveAll(tmpdir); err != nil {
			t.Fatal(err)
		}
	}()

	files, err := os.ReadDir(tmpdir)
	if err != nil {
		t.Fatal(err)
	}

	foundFiles := []string{}
	for _, file := range files {
		foundFiles = append(foundFiles, file.Name())
	}

	expectedFiles := []string{
		".dockerignore",
		"Dockerfile",
		"barfile",
		"symlink",
	}

	if !reflect.DeepEqual(expectedFiles, foundFiles) {
		t.Errorf(
			"BuildImage: incorrect files sent in tarball to docker server\nexpected %+v, found %+v",
			expectedFiles, foundFiles,
		)
	}
}

func TestBuildImageSendXRegistryConfig(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	opts := BuildImageOptions{
		Name:                "testImage",
		NoCache:             true,
		SuppressOutput:      true,
		RmTmpContainer:      true,
		ForceRmTmpContainer: true,
		OutputStream:        &buf,
		ContextDir:          "testing/data",
		AuthConfigs: AuthConfigurations{
			Configs: map[string]AuthConfiguration{
				"quay.io": {
					Username:      "foo",
					Password:      "bar",
					Email:         "baz",
					ServerAddress: "quay.io",
				},
			},
		},
	}

	encodedConfig := "eyJjb25maWdzIjp7InF1YXkuaW8iOnsidXNlcm5hbWUiOiJmb28iLCJwYXNzd29yZCI6ImJhciIsImVtYWlsIjoiYmF6Iiwic2VydmVyYWRkcmVzcyI6InF1YXkuaW8ifX19"
	if err := client.BuildImage(opts); err != nil {
		t.Fatal(err)
	}

	xRegistryConfig := fakeRT.requests[0].Header.Get("X-Registry-Config")
	if xRegistryConfig != encodedConfig {
		t.Errorf(
			"BuildImage: X-Registry-Config not set currectly: expected %q, got %q",
			encodedConfig,
			xRegistryConfig,
		)
	}
}

func unpackBodyTarball(req io.Reader) (tmpdir string, err error) {
	tmpdir, err = os.MkdirTemp("", "go-dockerclient-test")
	if err != nil {
		return
	}
	err = archive.Untar(req, tmpdir, &archive.TarOptions{
		Compression: archive.Uncompressed,
		NoLchown:    true,
	})
	return
}
