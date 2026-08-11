package provider

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LocalModel is a model available on this machine and the OpenAI-compatible
// endpoint that can serve it (llama-server, Ollama, or disk .gguf files).
type LocalModel struct {
	Name    string
	BaseURL string
}

type modelsResp struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

type ollamaTags struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// DefaultOllamaURL is Ollama's OpenAI-compatible endpoint.
const DefaultOllamaURL = "http://localhost:11434/v1"

// DiscoverLocalModels returns models available on this machine:
// the model(s) loaded by a running llama-server, models served by a
// running Ollama, and .gguf files found in common model directories.
// Results are de-duplicated by name.
func DiscoverLocalModels(baseURL string) []LocalModel {
	seen := map[string]bool{}
	var out []LocalModel

	add := func(m LocalModel) {
		m.Name = strings.TrimSpace(m.Name)
		m.Name = strings.TrimPrefix(m.Name, ":latest")
		if m.Name == "" || seen[m.Name] {
			return
		}
		seen[m.Name] = true
		out = append(out, m)
	}

	// 1. Running llama-server / local OpenAI-compatible server.
	if baseURL != "" {
		base := strings.TrimSuffix(baseURL, "/")
		if ids := serverModels(base + "/models"); len(ids) > 0 {
			for _, id := range ids {
				add(LocalModel{Name: id, BaseURL: base})
			}
		}
	}

	// 2. Running Ollama (OpenAI-compatible endpoint).
	if ids := serverModels(DefaultOllamaURL + "/models"); len(ids) > 0 {
		for _, id := range ids {
			add(LocalModel{Name: id, BaseURL: DefaultOllamaURL})
		}
	} else if names := ollamaNames(); len(names) > 0 {
		for _, n := range names {
			add(LocalModel{Name: n, BaseURL: DefaultOllamaURL})
		}
	}

	// 3. .gguf files in common model directories.
	for _, root := range localModelRoots() {
		collectGGUF(root, 0, func(base string) {
			add(LocalModel{Name: base, BaseURL: strings.TrimSuffix(baseURL, "/")})
		})
	}

	if len(out) == 0 {
		out = append(out,
			LocalModel{Name: "gemma-2-2b-it", BaseURL: strings.TrimSuffix(baseURL, "/")},
			LocalModel{Name: "llama-3.2-3b-instruct", BaseURL: strings.TrimSuffix(baseURL, "/")},
		)
	}
	return out
}

func serverModels(url string) []string {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var mr modelsResp
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil
	}
	var ids []string
	for _, m := range mr.Data {
		ids = append(ids, m.ID)
	}
	return ids
}

func ollamaNames() []string {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var t ollamaTags
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil
	}
	var names []string
	for _, m := range t.Models {
		names = append(names, m.Name)
	}
	return names
}

func localModelRoots() []string {
	home, _ := os.UserHomeDir()
	var roots []string
	candidates := []string{
		filepath.Join(home, "Downloads"),
		filepath.Join(home, "Desktop"),
		filepath.Join(home, "Documents"),
		filepath.Join(home, "Models"),
		filepath.Join(home, "models"),
		filepath.Join(home, ".lmstudio", "models"),
		filepath.Join(home, ".cache", "llama.cpp"),
		filepath.Join(home, ".llama"),
		filepath.Join(home, "models-llama"),
	}
	if home != "" {
		candidates = append(candidates, home)
	}
	for _, d := range candidates {
		if d != "" && dirExists(d) {
			roots = append(roots, d)
		}
	}
	if la, err := os.UserCacheDir(); err == nil {
		if d := filepath.Join(la, "nomic.ai", "GPT4All"); dirExists(d) {
			roots = append(roots, d)
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, cwd)
	}
	return roots
}

func dirExists(d string) bool {
	fi, err := os.Stat(d)
	return err == nil && fi.IsDir()
}

var skipDirs = map[string]bool{
	"AppData":     true,
	"node_modules": true,
	".git":        true,
	".venv":       true,
	"venv":        true,
	"__pycache__": true,
	"miniconda3":  true,
	"anaconda3":   true,
	".ollama":     true,
}

func collectGGUF(root string, depth int, add func(string)) {
	if depth >= 4 {
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if skipDirs[name] {
				continue
			}
			collectGGUF(filepath.Join(root, name), depth+1, add)
			continue
		}
		if strings.HasSuffix(strings.ToLower(name), ".gguf") {
			base := strings.TrimSuffix(name, filepath.Ext(name))
			add(base)
		}
	}
}
