package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alexandrmotologa/embshift"
	"github.com/alexandrmotologa/embshift/internal/adapter"
	"github.com/alexandrmotologa/embshift/internal/backfill"
	"github.com/alexandrmotologa/embshift/internal/config"
	"github.com/alexandrmotologa/embshift/internal/models"
	"github.com/alexandrmotologa/embshift/internal/proxy"
	"github.com/alexandrmotologa/embshift/internal/queue"
	"github.com/alexandrmotologa/embshift/internal/server"
	"github.com/alexandrmotologa/embshift/internal/shadow"
)

var version = "1.0.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "start":
		runStart(os.Args[2:])
	case "demo":
		runDemo(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("embshift v%s (zero-CGO Go runtime, embedded React 19 cockpit)\n", version)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("embshift: Zero-Downtime Vector Database Migration & Shadow Re-Embedding Engine")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  embshift <command> [flags]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  demo      Run self-contained simulation with synthetic vectors and embedded studio")
	fmt.Println("  start     Start production dual-writing proxy and migration orchestrator")
	fmt.Println("  version   Display engine version")
	fmt.Println("")
	fmt.Println("Flags for 'start' and 'demo':")
	fmt.Println("  --config <path>   Path to YAML configuration file (default: embshift.yaml)")
	fmt.Println("  --port <int>      Port for vector ingestion proxy (default: 8080)")
	fmt.Println("  --studio <int>    Port for React 19 Migration Studio (default: 8787)")
}

func runStart(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to YAML configuration file")
	port := fs.Int("port", 0, "Proxy ingestion port override")
	studioPort := fs.Int("studio", 0, "Migration studio port override")
	_ = fs.Parse(args)

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	if *port > 0 {
		cfg.Proxy.Port = *port
	}
	if *studioPort > 0 {
		cfg.Proxy.StudioPort = *studioPort
	}

	runEngine(cfg, false)
}

func runDemo(args []string) {
	fs := flag.NewFlagSet("demo", flag.ExitOnError)
	configPath := fs.String("config", "", "Optional configuration path")
	port := fs.Int("port", 8080, "Proxy port")
	studioPort := fs.Int("studio", 8787, "Studio cockpit port")
	_ = fs.Parse(args)

	cfg := config.DefaultConfig()
	if *configPath != "" {
		if loaded, err := config.LoadConfig(*configPath); err == nil {
			cfg = *loaded
		}
	}
	cfg.Legacy.Type = "mock"
	cfg.Target.Type = "mock"
	cfg.Embedder.Legacy.Type = "mock"
	cfg.Embedder.Target.Type = "mock"
	cfg.Proxy.Port = *port
	cfg.Proxy.StudioPort = *studioPort
	cfg.Storage.SQLitePath = "embshift_demo.db"

	runEngine(&cfg, true)
}

func runEngine(cfg *config.Config, isDemo bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Printf("[embshift] initializing SQLite replication queue at %s", cfg.Storage.SQLitePath)
	q, err := queue.NewSQLiteQueue(cfg.Storage.SQLitePath)
	if err != nil {
		log.Fatalf("failed to initialize sqlite queue: %v", err)
	}
	defer q.Close()

	// Initialize vector stores and embedders
	legacyStore, targetStore := createVectorStores(cfg)
	legacyEmbedder, targetEmbedder := createEmbedders(cfg)

	router := proxy.NewRouter(cfg, legacyStore, targetStore, legacyEmbedder, targetEmbedder, q)
	dualWriter := proxy.NewDualWriter(router, q)
	queryMirror := proxy.NewQueryMirror(router, cfg, q)
	orchestrator := backfill.NewOrchestrator(cfg, legacyStore, targetStore, targetEmbedder, q, router)
	qualityGate := shadow.NewQualityGate(cfg, q)

	// Start background replication worker
	go dualWriter.StartReplicationWorker(ctx)

	// In demo mode, seed initial documents and background live activity
	if isDemo {
		seedDemoCorpus(ctx, legacyStore, legacyEmbedder)
		go runDemoTrafficGenerator(ctx, dualWriter, queryMirror)
	}

	// Initialize HTTP server
	srv := server.NewServer(cfg, router, dualWriter, queryMirror, orchestrator, qualityGate, q)

	// Attach embedded React 19 UI
	if distFS, err := embshift.DistFS(); err == nil {
		srv.SetStaticFS(distFS)
		log.Printf("[embshift] embedded React 19 cockpit loaded")
	} else {
		log.Printf("[embshift] warning: embedded web dist not available: %v", err)
	}

	// Start telemetry broadcaster
	go srv.StartBroadcasting(ctx)

	handler := srv.Handler()

	// Start Ingestion Proxy HTTP server
	proxyAddr := fmt.Sprintf("%s:%d", cfg.Proxy.Host, cfg.Proxy.Port)
	proxyServer := &http.Server{
		Addr:    proxyAddr,
		Handler: handler,
	}

	go func() {
		log.Printf("[embshift] Vector Data Plane Proxy listening at http://%s", proxyAddr)
		if err := proxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("proxy server error: %v", err)
		}
	}()

	// If StudioPort is distinct, also listen on studio port
	var studioServer *http.Server
	if cfg.Proxy.StudioPort != cfg.Proxy.Port && cfg.Proxy.StudioPort > 0 {
		studioAddr := fmt.Sprintf("%s:%d", cfg.Proxy.Host, cfg.Proxy.StudioPort)
		studioServer = &http.Server{
			Addr:    studioAddr,
			Handler: handler,
		}
		go func() {
			log.Printf("[embshift] React 19 Migration Studio running at http://%s", studioAddr)
			if err := studioServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("studio server error: %v", err)
			}
		}()
	}

	fmt.Println("----------------------------------------------------------------")
	fmt.Printf("  embshift v%s started\n", version)
	fmt.Printf("  * Ingestion Proxy:  http://localhost:%d\n", cfg.Proxy.Port)
	fmt.Printf("  * Migration Studio: http://localhost:%d\n", cfg.Proxy.StudioPort)
	fmt.Println("  Press Ctrl+C to stop")
	fmt.Println("----------------------------------------------------------------")

	// Wait for OS termination signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Printf("[embshift] shutting down gracefully...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	_ = proxyServer.Shutdown(shutdownCtx)
	if studioServer != nil {
		_ = studioServer.Shutdown(shutdownCtx)
	}
	log.Printf("[embshift] shutdown complete.")
}

func createVectorStores(cfg *config.Config) (adapter.VectorStore, adapter.VectorStore) {
	var legacyStore adapter.VectorStore
	var targetStore adapter.VectorStore

	switch cfg.Legacy.Type {
	case "qdrant":
		legacyStore = adapter.NewQdrantStore(cfg.Legacy.Endpoint, cfg.Legacy.APIKey, cfg.Legacy.Collection, cfg.Legacy.Dimensions)
	case "pinecone":
		legacyStore = adapter.NewPineconeStore(cfg.Legacy.Endpoint, cfg.Legacy.APIKey, cfg.Legacy.Collection, cfg.Legacy.Dimensions)
	case "pgvector":
		legacyStore = adapter.NewPgVectorStore(cfg.Legacy.Endpoint, cfg.Legacy.APIKey, cfg.Legacy.Collection, cfg.Legacy.Dimensions)
	default:
		legacyStore = adapter.NewMockStore("legacy-"+cfg.Legacy.Collection, cfg.Legacy.Dimensions)
	}

	switch cfg.Target.Type {
	case "qdrant":
		targetStore = adapter.NewQdrantStore(cfg.Target.Endpoint, cfg.Target.APIKey, cfg.Target.Collection, cfg.Target.Dimensions)
	case "pinecone":
		targetStore = adapter.NewPineconeStore(cfg.Target.Endpoint, cfg.Target.APIKey, cfg.Target.Collection, cfg.Target.Dimensions)
	case "pgvector":
		targetStore = adapter.NewPgVectorStore(cfg.Target.Endpoint, cfg.Target.APIKey, cfg.Target.Collection, cfg.Target.Dimensions)
	default:
		targetStore = adapter.NewMockStore("target-"+cfg.Target.Collection, cfg.Target.Dimensions)
	}

	return legacyStore, targetStore
}

func createEmbedders(cfg *config.Config) (adapter.Embedder, adapter.Embedder) {
	var legacyEmbedder adapter.Embedder
	var targetEmbedder adapter.Embedder

	if cfg.Embedder.Legacy.Type == "openai" || cfg.Embedder.Legacy.Type == "voyage" {
		legacyEmbedder = adapter.NewHTTPEmbedder(cfg.Embedder.Legacy.Endpoint, cfg.Embedder.Legacy.APIKey, cfg.Embedder.Legacy.Model, cfg.Embedder.Legacy.Dimensions)
	} else {
		legacyEmbedder = adapter.NewMockEmbedder(cfg.Embedder.Legacy.Model, cfg.Embedder.Legacy.Dimensions)
	}

	if cfg.Embedder.Target.Type == "openai" || cfg.Embedder.Target.Type == "voyage" {
		targetEmbedder = adapter.NewHTTPEmbedder(cfg.Embedder.Target.Endpoint, cfg.Embedder.Target.APIKey, cfg.Embedder.Target.Model, cfg.Embedder.Target.Dimensions)
	} else {
		targetEmbedder = adapter.NewMockEmbedder(cfg.Embedder.Target.Model, cfg.Embedder.Target.Dimensions)
	}

	legacyEmbedder.SetPrefixes(cfg.Embedder.Legacy.DocumentPrefix, cfg.Embedder.Legacy.QueryPrefix)
	targetEmbedder.SetPrefixes(cfg.Embedder.Target.DocumentPrefix, cfg.Embedder.Target.QueryPrefix)

	return legacyEmbedder, targetEmbedder
}

func seedDemoCorpus(ctx context.Context, store adapter.VectorStore, embedder adapter.Embedder) {
	log.Printf("[demo] seeding synthetic production vector corpus (100 documents)")
	topics := []string{
		"Distributed vector indexing algorithms and graph partitioning",
		"Zero downtime database migration patterns in distributed systems",
		"Retrieval augmented generation with dense embedding models",
		"HNSW graph connectivity and approximate nearest neighbor search",
		"Transactional idempotency gates in dual writing proxies",
		"Memory footprint optimization in high concurrency Go runtimes",
		"Cosine distance metrics and inner product vector quantization",
		"Rate limiting token buckets and API expenditure guardrails",
		"Shadow traffic evaluation with Jaccard and NDCG ranking deltas",
		"Atomic pointer swapping for instantaneous traffic cutover",
	}

	var records []models.VectorRecord
	for i := 1; i <= 100; i++ {
		topic := topics[i%len(topics)]
		text := fmt.Sprintf("%s (document chunk ID #%03d)", topic, i)
		vec, _, _ := embedder.EmbedQuery(ctx, text)

		records = append(records, models.VectorRecord{
			ID:   fmt.Sprintf("doc-%03d", i),
			Text: text,
			Vector: vec,
			Payload: map[string]any{
				"category": "technical-whitepaper",
				"chunk_id": i,
				"topic":    topic,
			},
			CreatedAt: time.Now().Add(-time.Duration(100-i) * time.Minute),
		})
	}

	_ = store.Upsert(ctx, records)
	log.Printf("[demo] successfully seeded 100 legacy vector documents")
}

func runDemoTrafficGenerator(ctx context.Context, dw *proxy.DualWriter, qm *proxy.QueryMirror) {
	queryPhrases := []string{
		"dense vector indexing and graph search",
		"dual writing database migration with zero downtime",
		"retrieval augmented generation accuracy metrics",
		"approximate nearest neighbor performance in memory",
		"token rate limiting and spend budget controls",
	}

	queryTicker := time.NewTicker(3 * time.Second)
	defer queryTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-queryTicker.C:
			phrase := queryPhrases[time.Now().Unix()%int64(len(queryPhrases))]
			_, _ = qm.Query(ctx, models.SearchQuery{
				Text: phrase,
				TopK: 5,
			})
		}
	}
}
