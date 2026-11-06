// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const clientUsage = `Usage: diffluence client <command>

Commands:
  spaces list                 list the spaces you can see, as JSON
  pages list <space-id>       list the pages of a space, as JSON
  export <space-id> <file>    save a space as a zip
  import <space-id> <file>    import a file into a space
  backup-content <folder>     save every space you can see as a zip

Set DIFFLUENCE_URL, and the token in DIFFLUENCE_TOKEN or a file named by
DIFFLUENCE_TOKEN_FILE. The token is never an argument, so it stays out of the list
of processes. JSON goes to standard output, messages to standard error.
`

// The exit codes of the client.
const (
	exitDone    = 0
	exitFailed  = 1
	exitUsage   = 2
	exitRefused = 3
)

const clientTimeout = 10 * time.Minute

// errRefused is a server saying no to this token.
var errRefused = errors.New("the server refused this token")

// apiClient only uses the public API (PLT-040).
type apiClient struct {
	base  string
	token string
	http  *http.Client
}

func newAPIClient(env func(string) string) (*apiClient, error) {
	base := strings.TrimRight(env("DIFFLUENCE_URL"), "/")
	if base == "" {
		return nil, errors.New("set DIFFLUENCE_URL")
	}
	token := env("DIFFLUENCE_TOKEN")
	if file := env("DIFFLUENCE_TOKEN_FILE"); token == "" && file != "" {
		raw, err := os.ReadFile(file) // #nosec G304 -- the person running this names their own file
		if err != nil {
			return nil, fmt.Errorf("read the token file: %w", err)
		}
		token = strings.TrimSpace(string(raw))
	}
	if token == "" {
		return nil, errors.New("set DIFFLUENCE_TOKEN or DIFFLUENCE_TOKEN_FILE")
	}
	return &apiClient{base: base, token: token, http: &http.Client{Timeout: clientTimeout}}, nil
}

func (c *apiClient) call(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 300 {
		return resp, nil
	}
	defer func() { _ = resp.Body.Close() }()
	said, err := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%w: %s", errRefused, said)
	}
	return nil, fmt.Errorf("the server answered %d: %s", resp.StatusCode, said)
}

func (c *apiClient) copyTo(ctx context.Context, path string, w io.Writer) error {
	resp, err := c.call(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, err = io.Copy(w, resp.Body)
	return err
}

func (c *apiClient) saveTo(ctx context.Context, path, file string) error {
	out, err := os.Create(file) // #nosec G304 -- the person running this names their own file
	if err != nil {
		return err
	}
	if err := c.copyTo(ctx, path, out); err != nil {
		_ = out.Close()
		// Half a backup that looks whole is worse than none.
		return errors.Join(err, os.Remove(file))
	}
	return out.Close()
}

func (c *apiClient) importFile(ctx context.Context, space, file string, stdout io.Writer) error {
	in, err := os.Open(file) // #nosec G304 -- the person running this names their own file
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	path := "/api/v1/spaces/" + url.PathEscape(space) + "/imports?filename=" + url.QueryEscape(filepath.Base(file))
	resp, err := c.call(ctx, http.MethodPost, path, in)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, err = io.Copy(stdout, resp.Body)
	return err
}

func (c *apiClient) backup(ctx context.Context, folder string, stderr io.Writer) error {
	resp, err := c.call(ctx, http.MethodGet, "/api/v1/spaces", nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	var list struct {
		Spaces []struct {
			ID  string `json:"id"`
			Key string `json:"key"`
		} `json:"spaces"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return fmt.Errorf("read the list of spaces: %w", err)
	}
	if err := os.MkdirAll(folder, 0o750); err != nil {
		return err
	}
	for _, s := range list.Spaces {
		name := filepath.Join(folder, filepath.Base(s.Key)+".zip")
		if err := c.saveTo(ctx, "/api/v1/spaces/"+url.PathEscape(s.ID)+"/export.zip", name); err != nil {
			return fmt.Errorf("space %s: %w", s.Key, err)
		}
		fmt.Fprintln(stderr, "saved", name)
	}
	return nil
}

func (c *apiClient) run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	command := strings.Join(args[:min(2, len(args))], " ")
	switch {
	case command == "spaces list" && len(args) == 2:
		return c.copyTo(ctx, "/api/v1/spaces", stdout)
	case command == "pages list" && len(args) == 3:
		return c.copyTo(ctx, "/api/v1/spaces/"+url.PathEscape(args[2])+"/tree", stdout)
	case args[0] == "export" && len(args) == 3:
		return c.saveTo(ctx, "/api/v1/spaces/"+url.PathEscape(args[1])+"/export.zip", args[2])
	case args[0] == "import" && len(args) == 3:
		return c.importFile(ctx, args[1], args[2], stdout)
	case args[0] == "backup-content" && len(args) == 2:
		return c.backup(ctx, args[1], stderr)
	}
	return errClientUsage
}

var errClientUsage = errors.New("usage")

// cmdClient runs the command line tool, and answers with its exit code.
func cmdClient(ctx context.Context, args []string, env func(string) string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, clientUsage)
		return exitUsage
	}
	c, err := newAPIClient(env)
	if err != nil {
		fmt.Fprintln(stderr, "diffluence client:", err)
		return exitUsage
	}
	err = c.run(ctx, args, stdout, stderr)
	switch {
	case err == nil:
		return exitDone
	case errors.Is(err, errClientUsage):
		fmt.Fprint(stderr, clientUsage)
		return exitUsage
	case errors.Is(err, errRefused):
		fmt.Fprintln(stderr, "diffluence client:", err)
		return exitRefused
	}
	fmt.Fprintln(stderr, "diffluence client:", err)
	return exitFailed
}
