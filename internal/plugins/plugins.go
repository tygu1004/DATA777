// Package plugins implements the extension contract from docs/plugins.md: operators (an
// action run against a selection) and panels (a registered UI surface), both reverse-proxied
// through data777's own server so a plugin only ever needs to be reachable from the core
// server, never from the browser.
package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type Operator struct {
	Name      string          `json:"name" yaml:"name"`
	Label     string          `json:"label" yaml:"label"`
	Selection string          `json:"selection" yaml:"selection"` // required | optional | none
	Inputs    json.RawMessage `json:"inputs,omitempty" yaml:"inputs,omitempty"`
}

type Panel struct {
	Name   string   `json:"name" yaml:"name"`
	Label  string   `json:"label" yaml:"label"`
	Mounts []string `json:"mounts" yaml:"mounts"`
}

type Manifest struct {
	Name      string     `json:"name"`
	Operators []Operator `json:"operators"`
	Panels    []Panel    `json:"panels"`
}

type configEntry struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type config struct {
	Plugins []configEntry `yaml:"plugins"`
}

// Registry holds the statically configured plugin list (docs/plugins.md#registration:
// plugins register via a config file an admin edits, never by self-registering at runtime)
// and the manifests fetched from each one that was reachable at the last reload.
type Registry struct {
	configPath string
	client     *http.Client

	mu        sync.RWMutex
	entries   []configEntry
	manifests map[string]Manifest
}

// Load reads the plugin config file. A missing file means no plugins are configured — that's
// the default, not an error, matching every other optional attachment in this project.
func Load(configPath string) (*Registry, error) {
	r := &Registry{configPath: configPath, client: &http.Client{Timeout: 30 * time.Second}, manifests: map[string]Manifest{}}

	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return r, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read plugins config: %w", err)
	}
	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse plugins config: %w", err)
	}
	r.entries = cfg.Plugins
	return r, nil
}

// Reload re-fetches every configured plugin's manifest. A plugin that's unreachable is logged
// and skipped, not a startup failure (docs/plugins.md#registration).
func (r *Registry) Reload(ctx context.Context) {
	manifests := map[string]Manifest{}
	r.mu.RLock()
	entries := r.entries
	r.mu.RUnlock()

	for _, e := range entries {
		m, err := r.fetchManifest(ctx, e)
		if err != nil {
			log.Printf("plugin %q at %s: %v (skipped)", e.Name, e.URL, err)
			continue
		}
		manifests[e.Name] = m
	}

	r.mu.Lock()
	r.manifests = manifests
	r.mu.Unlock()
}

func (r *Registry) fetchManifest(ctx context.Context, e configEntry) (Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(e.URL, "/")+"/data777-plugin.json", nil)
	if err != nil {
		return Manifest{}, err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return Manifest{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Manifest{}, fmt.Errorf("manifest fetch returned %d", resp.StatusCode)
	}
	var m Manifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if m.Name == "" {
		m.Name = e.Name
	}
	return m, nil
}

func (r *Registry) Manifests() map[string]Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]Manifest, len(r.manifests))
	for k, v := range r.manifests {
		out[k] = v
	}
	return out
}

func (r *Registry) urlFor(name string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.entries {
		if e.Name == name {
			return e.URL, true
		}
	}
	return "", false
}

func (r *Registry) findOperator(pluginName, operatorName string) (Operator, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.manifests[pluginName]
	if !ok {
		return Operator{}, false
	}
	for _, op := range m.Operators {
		if op.Name == operatorName {
			return op, true
		}
	}
	return Operator{}, false
}

// RunOperator posts the request body (selection + inputs) to a plugin's operator endpoint,
// with a bearer token attached so the operator can call back into data777's own API — this is
// the whole write path: an operator is an SDK script with a declared form and a trigger
// button, not a special integration surface (docs/plugins.md).
//
// ponytail: the call is a single synchronous HTTP round trip, awaited inside the job's own
// goroutine — progress is reported start/finish, not via a live callback channel from the
// plugin back into the job. A push-progress endpoint is real scope, not free; add one if a
// slow operator (e.g. embedding computation) actually needs finer-grained progress than
// "started" / "done".
func (r *Registry) RunOperator(ctx context.Context, pluginName, operatorName string, body []byte, token string) (json.RawMessage, error) {
	if _, ok := r.findOperator(pluginName, operatorName); !ok {
		return nil, fmt.Errorf("unknown operator %s/%s", pluginName, operatorName)
	}
	base, _ := r.urlFor(pluginName)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/operators/"+operatorName, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call operator %s/%s: %w", pluginName, operatorName, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read operator response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("operator %s/%s returned %d: %s", pluginName, operatorName, resp.StatusCode, raw)
	}
	return json.RawMessage(raw), nil
}

// PanelProxy returns a reverse proxy handler for a plugin's panel UI, so the iframe src the
// dashboard uses is always same-origin (docs/plugins.md#data777-side-endpoints).
func (r *Registry) PanelProxy(pluginName string) (http.Handler, error) {
	base, ok := r.urlFor(pluginName)
	if !ok {
		return nil, fmt.Errorf("unknown plugin %q", pluginName)
	}
	target, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("invalid plugin url: %w", err)
	}
	return httputil.NewSingleHostReverseProxy(target), nil
}
