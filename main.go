package main

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"

	"github.com/Harshit-Dhundale/helm-image-analyzer/internal/chart"
	"github.com/Harshit-Dhundale/helm-image-analyzer/internal/kube"
	"github.com/Harshit-Dhundale/helm-image-analyzer/internal/registry"
)

type AnalyzeRequest struct {
	
	ChartURL   string            `json:"chart_url,omitempty"`   
	LocalPath  string            `json:"local_path,omitempty"`  
	Ref        string            `json:"ref,omitempty"`         
	Subpath    string            `json:"subpath,omitempty"`    
	ValuesYAML string            `json:"values_yaml,omitempty"`
	Set        map[string]string `json:"set,omitempty"`         

	// Image options
	Platform string `json:"platform,omitempty"` 
	Download bool   `json:"download,omitempty"`
	DownloadDir string `json:"download_dir,omitempty"`
}

type ImageInfo struct {
	Image       string `json:"image"`
	ResolvedRef string `json:"resolved_ref"`
	Digest      string `json:"digest"`
	Layers      int    `json:"layers"`
	SizeBytes   int64  `json:"size_bytes"`
	SizeHuman   string `json:"size_human"`
	Platform    string `json:"platform"`
	Downloaded  string `json:"downloaded,omitempty"`
	Error       string `json:"error,omitempty"`
}

type AnalyzeResponse struct {
	Chart struct {
		Source   string `json:"source"`
		Ref      string `json:"ref,omitempty"`
		Subpath  string `json:"subpath,omitempty"`
		Rendered int    `json:"rendered_documents"`
	} `json:"chart"`
	Images []ImageInfo `json:"images"`
}

func main() {
	log := zerolog.New(os.Stdout).With().Timestamp().Logger()

	r := mux.NewRouter()
	r.Use(hlog.NewHandler(log))
	r.Use(hlog.RequestIDHandler("req_id", "X-Request-ID"))
	r.Use(hlog.AccessHandler(func(r *http.Request, status, size int, duration time.Duration) {
		hlog.FromRequest(r).Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", status).
			Int("size", size).
			Dur("duration", duration).
			Msg("request")
	}))
	r.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}).Methods("GET")

	r.HandleFunc("/analyze", analyzeHandler).Methods("POST")

	addr := ":" + getenv("PORT", "8080")
	log.Info().Msgf("listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal().Err(err).Send()
	}
}
func getenv(k, def string) string { if v := os.Getenv(k); v != "" { return v }; return def }


func analyzeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := hlog.FromRequest(r)

	var req AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.ChartURL == "" && req.LocalPath == "" {
		http.Error(w, "provide chart_url or local_path", http.StatusBadRequest)
		return
	}


	work, meta, cleanup, err := chart.FetchChart(ctx, req.ChartURL, req.LocalPath, req.Ref, req.Subpath)
	if err != nil {
		http.Error(w, "fetch chart failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer cleanup()


	renderOut, docCount, err := chart.RenderWithHelm(ctx, work.ChartDir, req.ValuesYAML, req.Set)
	if err != nil {
		http.Error(w, "helm template failed: "+err.Error(), http.StatusBadRequest)
		return
	}


	images, err := kube.ExtractImages(renderOut)
	if err != nil {
		http.Error(w, "parse rendered YAML failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(images) == 0 {
		images = []string{}
	}


	platform := req.Platform
	if platform == "" {
		platform = "linux/amd64"
	}

	results := make([]ImageInfo, 0, len(images))
	cache := map[string]ImageInfo{} 
	for _, img := range images {
		if v, ok := cache[img+"|"+platform]; ok {
			results = append(results, v)
			continue
		}
		info, err := registry.InspectImage(ctx, img, platform, req.Download, req.DownloadDir)
		if err != nil {
			
			results = append(results, ImageInfo{
				Image:       img,
				ResolvedRef: "",
				Digest:      "", 
				Error: err.Error(),
				Layers:      0,
				SizeBytes:   0,
				SizeHuman:   "0 B",
				Platform:    platform,
			})
			continue
		}
		conv := ImageInfo{
    Image:       info.Image,
    ResolvedRef: info.ResolvedRef,
    Digest:      info.Digest,
    Layers:      info.Layers,
    SizeBytes:   info.SizeBytes,
    SizeHuman:   info.SizeHuman,
    Platform:    info.Platform,
    Downloaded:  info.Downloaded,
}

cache[img+"|"+platform] = conv
results = append(results, conv)
	}

	resp := AnalyzeResponse{}
	resp.Chart.Source = meta.Source
	resp.Chart.Ref = meta.Ref
	resp.Chart.Subpath = meta.Subpath
	resp.Chart.Rendered = docCount
	resp.Images = results

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)

	logger.Info().Int("images", len(results)).Msg("analyze complete")
}
