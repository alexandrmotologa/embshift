package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config encapsulates all operational parameters for the embshift migration engine.
type Config struct {
	Proxy    ProxyConfig    `yaml:"proxy"`
	Legacy   StoreConfig    `yaml:"legacy"`
	Target   StoreConfig    `yaml:"target"`
	Embedder EmbedderConfig `yaml:"embedder"`
	Backfill BackfillConfig `yaml:"backfill"`
	Shadow   ShadowConfig   `yaml:"shadow"`
	Cutover  CutoverConfig  `yaml:"cutover"`
	Storage  StorageConfig  `yaml:"storage"`
}

// ProxyConfig defines network bindings for vector ingestion and migration studio.
type ProxyConfig struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	StudioPort int    `yaml:"studio_port"`
}

// StoreConfig specifies connection and indexing details for a vector store.
type StoreConfig struct {
	Type       string `yaml:"type"` // "mock", "qdrant", "pgvector", "pinecone", "milvus"
	Endpoint   string `yaml:"endpoint"`
	APIKey     string `yaml:"api_key"`
	Collection string `yaml:"collection"`
	Dimensions int    `yaml:"dimensions"`
	Model      string `yaml:"model"`
}

// EmbedderConfig specifies embedding models for legacy and target stores.
type EmbedderConfig struct {
	Legacy EmbedderModelConfig `yaml:"legacy"`
	Target EmbedderModelConfig `yaml:"target"`
}

// EmbedderModelConfig specifies API access parameters for an embedding model.
type EmbedderModelConfig struct {
	Type                     string  `yaml:"type"` // "mock", "openai", "voyage", "ollama"
	Endpoint                 string  `yaml:"endpoint"`
	APIKey                   string  `yaml:"api_key"`
	Model                    string  `yaml:"model"`
	Dimensions               int     `yaml:"dimensions"`
	PricePerMillionTokensUSD float64 `yaml:"price_per_million_tokens_usd"`
	DocumentPrefix           string  `yaml:"document_prefix"` // e.g. "passage: " or "Represent the document for retrieval: "
	QueryPrefix              string  `yaml:"query_prefix"`    // e.g. "query: " or "Represent the query: "
}

// BackfillConfig tunes batch processing, concurrency, and token spend limits.
type BackfillConfig struct {
	BatchSize         int     `yaml:"batch_size"`
	Concurrency       int     `yaml:"concurrency"`
	RequestsPerMinute int     `yaml:"requests_per_minute"`
	TokensPerMinute   int     `yaml:"tokens_per_minute"`
	MaxBudgetUSD      float64 `yaml:"max_budget_usd"`
	TextField         string  `yaml:"text_field"`
	MaxRetryAttempts  int     `yaml:"max_retry_attempts"` // After N attempts, move to Dead-Letter Queue
}

// ShadowConfig controls production query mirroring and quality verification.
type ShadowConfig struct {
	SampleRatio         float64 `yaml:"sample_ratio"`
	TopK                int     `yaml:"top_k"`
	MinJaccardThreshold float64 `yaml:"min_jaccard_threshold"`
	MaxLatencyDeltaMs   float64 `yaml:"max_latency_delta_ms"`
}

// CutoverConfig specifies safety thresholds required before traffic switching.
type CutoverConfig struct {
	RequireParityRatio       float64 `yaml:"require_parity_ratio"`
	MaxReplicationLagMs      float64 `yaml:"max_replication_lag_ms"`
	RequireZeroPendingQueue  bool    `yaml:"require_zero_pending_queue"`
	EnableReverseReplication bool    `yaml:"enable_reverse_replication"`
	InitialCanaryWeight      float64 `yaml:"initial_canary_weight"` // Initial canary traffic (0.0 to 1.0)
	MaxCanaryErrorRate       float64 `yaml:"max_canary_error_rate"` // e.g. 0.05 (5%) triggers auto-abort
}

// StorageConfig specifies persistence settings for the local SQLite state database.
type StorageConfig struct {
	SQLitePath string `yaml:"sqlite_path"`
}

// DefaultConfig returns production-ready defaults.
func DefaultConfig() Config {
	return Config{
		Proxy: ProxyConfig{
			Host:       "0.0.0.0",
			Port:       8080,
			StudioPort: 8787,
		},
		Legacy: StoreConfig{
			Type:       "mock",
			Endpoint:   "http://localhost:6333",
			Collection: "production-vectors-v1",
			Dimensions: 1536,
			Model:      "text-embedding-ada-002",
		},
		Target: StoreConfig{
			Type:       "mock",
			Endpoint:   "http://localhost:6334",
			Collection: "production-vectors-v2",
			Dimensions: 3072,
			Model:      "text-embedding-3-large",
		},
		Embedder: EmbedderConfig{
			Legacy: EmbedderModelConfig{
				Type:                     "mock",
				Model:                    "text-embedding-ada-002",
				Dimensions:               1536,
				PricePerMillionTokensUSD: 0.10,
				DocumentPrefix:           "",
				QueryPrefix:              "",
			},
			Target: EmbedderModelConfig{
				Type:                     "mock",
				Model:                    "text-embedding-3-large",
				Dimensions:               3072,
				PricePerMillionTokensUSD: 0.13,
				DocumentPrefix:           "",
				QueryPrefix:              "",
			},
		},
		Backfill: BackfillConfig{
			BatchSize:         100,
			Concurrency:       4,
			RequestsPerMinute: 3000,
			TokensPerMinute:   1000000,
			MaxBudgetUSD:      50.0,
			TextField:         "text",
			MaxRetryAttempts:  3,
		},
		Shadow: ShadowConfig{
			SampleRatio:         0.25,
			TopK:                10,
			MinJaccardThreshold: 0.40,
			MaxLatencyDeltaMs:   150.0,
		},
		Cutover: CutoverConfig{
			RequireParityRatio:       1.0,
			MaxReplicationLagMs:      100.0,
			RequireZeroPendingQueue:  true,
			EnableReverseReplication: true,
			InitialCanaryWeight:      0.0,
			MaxCanaryErrorRate:       0.05,
		},
		Storage: StorageConfig{
			SQLitePath: "embshift.db",
		},
	}
}

// LoadConfig reads configuration from file or falls back to defaults with environment overrides.
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("unmarshal config yaml: %w", err)
		}
	}

	applyEnvOverrides(&cfg)
	return &cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if val := os.Getenv("EMBSHIFT_PROXY_PORT"); val != "" {
		if p, err := strconv.Atoi(val); err == nil {
			cfg.Proxy.Port = p
		}
	}
	if val := os.Getenv("EMBSHIFT_STUDIO_PORT"); val != "" {
		if p, err := strconv.Atoi(val); err == nil {
			cfg.Proxy.StudioPort = p
		}
	}
	if val := os.Getenv("EMBSHIFT_SQLITE_PATH"); val != "" {
		cfg.Storage.SQLitePath = val
	}
	if val := os.Getenv("EMBSHIFT_LEGACY_TYPE"); val != "" {
		cfg.Legacy.Type = val
	}
	if val := os.Getenv("EMBSHIFT_TARGET_TYPE"); val != "" {
		cfg.Target.Type = val
	}
	if val := os.Getenv("OPENAI_API_KEY"); val != "" {
		if cfg.Embedder.Target.APIKey == "" {
			cfg.Embedder.Target.APIKey = val
		}
	}
}
