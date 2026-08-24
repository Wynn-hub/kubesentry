package console

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

const apiDocsHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>KubeSentry API Explorer</title>
<style>body{font:14px system-ui;margin:2rem;max-width:1100px}code,pre{background:#f4f4f4;padding:.4rem}section{border:1px solid #ddd;border-radius:6px;margin:1rem 0;padding:1rem}small{color:#666}</style>
</head><body><h1>KubeSentry API Explorer</h1><p><small>OpenAPI 3.1 contract</small></p>
<div id="content">Loading specification…</div>
<script>
fetch('/api/docs/openapi.json').then(r=>r.json()).then(spec=>{
 const root=document.getElementById('content');
 root.innerHTML='';
 for(const [path,item] of Object.entries(spec.paths||{})) {
  const section=document.createElement('section');
  section.innerHTML='<h2>'+path+'</h2>';
  for(const [method,op] of Object.entries(item)) {
   if(method==='parameters') continue;
   const p=document.createElement('p');
   p.innerHTML='<code>'+method.toUpperCase()+'</code> '+(op.summary||op.description||'');
   section.appendChild(p);
  }
  root.appendChild(section);
 }
}).catch(err=>{document.getElementById('content').textContent='Unable to load OpenAPI specification: '+err});
</script></body></html>`

func (h *Handlers) openAPIDocs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(apiDocsHTML))
}

func (h *Handlers) openAPISpec(w http.ResponseWriter, _ *http.Request) {
	content, err := loadOpenAPISpec()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "load OpenAPI specification: "+err.Error())
		return
	}
	var document any
	if err := yaml.Unmarshal(content, &document); err != nil {
		writeErr(w, http.StatusInternalServerError, "parse OpenAPI specification: "+err.Error())
		return
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "encode OpenAPI specification: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}

func loadOpenAPISpec() ([]byte, error) {
	candidates := []string{}
	if configured := os.Getenv("OPENAPI_SPEC_FILE"); configured != "" {
		candidates = append(candidates, configured)
	}
	candidates = append(candidates,
		"/api/openapi.yaml",
		"api/openapi.yaml",
		"../api/openapi.yaml",
		"/etc/kubesentry/api/openapi.yaml",
	)
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), "api/openapi.yaml"))
	}
	var lastErr error
	for _, path := range candidates {
		content, err := os.ReadFile(path)
		if err == nil {
			return content, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no candidate paths configured")
	}
	return nil, lastErr
}
