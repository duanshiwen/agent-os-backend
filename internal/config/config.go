package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	App              AppConfig
	Database         DatabaseConfig
	Redis            RedisConfig
	JWT              JWTConfig
	CORS             CORSConfig
	Admission        AdmissionConfig
	IdentityVerifier IdentityVerifierConfig
	ObjectStorage    ObjectStorageConfig
	Embedding        EmbeddingConfig
	Background       BackgroundConfig
}

type AppConfig struct {
	Name, Env, Port string
	AutoMigrate     bool
}
type DatabaseConfig struct {
	Host, Port, User, Password, DBName, SSLMode string
	MaxOpenConns, MaxIdleConns, ConnMaxLifeMin  int
}
type RedisConfig struct {
	Host, Port, Password string
	DB                   int
}
type JWTConfig struct {
	Secret          string
	AccessTokenMins int
	Issuer          string
}
type CORSConfig struct{ AllowOrigins []string }
type AdmissionConfig struct{ PolicyType, InvitationCode string }
type IdentityVerifierConfig struct {
	FFILibraryPath string
}

type ObjectStorageConfig struct {
	Endpoint        string
	PublicEndpoint  string
	AccessKey       string
	SecretKey       string
	UseSSL          bool
	Bucket          string
	UploadTTLSecs   int
	DownloadTTLSecs int
}

type EmbeddingConfig struct {
	Provider     string
	Endpoint     string
	Model        string
	Dimensions   int
	TimeoutSecs  int
	MaxBatchSize int
}

type BackgroundConfig struct {
	RunOnStart                           bool
	OfflineMessageCleanupIntervalSeconds int
	SyncEventCleanupIntervalSeconds      int
	SensitiveConfirmationIntervalSeconds int
	KBSubscriptionExpiryIntervalSeconds  int
	EmbeddingWorkerEnabled               bool
	EmbeddingWorkerID                    string
	EmbeddingWorkerBatchSize             int
	EmbeddingWorkerPollIntervalSeconds   int
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=Asia/Shanghai", d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode)
}
func (r RedisConfig) Addr() string { return fmt.Sprintf("%s:%s", r.Host, r.Port) }

func Load() (*Config, error) {
	_ = godotenv.Load()
	env := getEnv("APP_ENV", "development")
	cfg := &Config{
		App:              AppConfig{Name: getEnv("APP_NAME", "agent-os-backend"), Env: env, Port: getEnv("APP_PORT", "8080"), AutoMigrate: getEnvBool("AUTO_MIGRATE", env != "production")},
		Database:         DatabaseConfig{Host: getEnv("PG_HOST", "localhost"), Port: getEnv("PG_PORT", "5432"), User: getEnv("PG_USER", "postgres"), Password: getEnv("PG_PASS", "postgres"), DBName: getEnv("PG_DB", "agent_os"), SSLMode: getEnv("PG_SSLMODE", "disable"), MaxOpenConns: 25, MaxIdleConns: 10, ConnMaxLifeMin: 30},
		Redis:            RedisConfig{Host: getEnv("REDIS_HOST", "localhost"), Port: getEnv("REDIS_PORT", "6379"), Password: getEnv("REDIS_PASS", ""), DB: getEnvInt("REDIS_DB", 0)},
		JWT:              JWTConfig{Secret: getEnv("JWT_SECRET", "dev-secret-change-me-use-48-plus-bytes-in-production"), AccessTokenMins: getEnvInt("JWT_ACCESS_TOKEN_MINS", 60), Issuer: "agent-os"},
		CORS:             CORSConfig{AllowOrigins: splitCSV(getEnv("CORS_ORIGIN", "http://localhost:5173"))},
		Admission:        AdmissionConfig{PolicyType: getEnv("ADMISSION_POLICY", "protocol"), InvitationCode: getEnv("ADMISSION_INVITATION_CODE", "agentos-dev")},
		IdentityVerifier: IdentityVerifierConfig{},
		ObjectStorage: ObjectStorageConfig{
			Endpoint:        getEnv("OBJECT_STORAGE_ENDPOINT", "localhost:9000"),
			PublicEndpoint:  getEnv("OBJECT_STORAGE_PUBLIC_ENDPOINT", getEnv("OBJECT_STORAGE_ENDPOINT", "localhost:9000")),
			AccessKey:       getEnv("OBJECT_STORAGE_ACCESS_KEY", "minioadmin"),
			SecretKey:       getEnv("OBJECT_STORAGE_SECRET_KEY", "minioadmin"),
			UseSSL:          getEnvBool("OBJECT_STORAGE_USE_SSL", false),
			Bucket:          getEnv("OBJECT_STORAGE_BUCKET", "agentos-objects"),
			UploadTTLSecs:   getEnvInt("OBJECT_UPLOAD_PRESIGN_TTL_SECONDS", 900),
			DownloadTTLSecs: getEnvInt("OBJECT_DOWNLOAD_PRESIGN_TTL_SECONDS", 900),
		},
		Embedding: EmbeddingConfig{
			Provider:     getEnv("EMBEDDING_PROVIDER", "disabled"),
			Endpoint:     getEnv("EMBEDDING_ENDPOINT", "http://localhost:8091"),
			Model:        getEnv("EMBEDDING_MODEL", "BAAI/bge-m3"),
			Dimensions:   getEnvInt("EMBEDDING_DIMENSIONS", 1024),
			TimeoutSecs:  getEnvInt("EMBEDDING_TIMEOUT_SECONDS", 30),
			MaxBatchSize: getEnvInt("EMBEDDING_MAX_BATCH_SIZE", 16),
		},
		Background: BackgroundConfig{
			RunOnStart:                           getEnvBool("BACKGROUND_RUN_ON_START", false),
			OfflineMessageCleanupIntervalSeconds: getEnvInt("BACKGROUND_OFFLINE_CLEANUP_INTERVAL_SECONDS", 3600),
			SyncEventCleanupIntervalSeconds:      getEnvInt("BACKGROUND_SYNC_CLEANUP_INTERVAL_SECONDS", 21600),
			SensitiveConfirmationIntervalSeconds: getEnvInt("BACKGROUND_SENSITIVE_CONFIRMATION_INTERVAL_SECONDS", 900),
			KBSubscriptionExpiryIntervalSeconds:  getEnvInt("BACKGROUND_KB_SUBSCRIPTION_EXPIRY_INTERVAL_SECONDS", 900),
			EmbeddingWorkerEnabled:               getEnvBool("BACKGROUND_EMBEDDING_WORKER_ENABLED", false),
			EmbeddingWorkerID:                    getEnv("BACKGROUND_EMBEDDING_WORKER_ID", ""),
			EmbeddingWorkerBatchSize:             getEnvInt("BACKGROUND_EMBEDDING_WORKER_BATCH_SIZE", 8),
			EmbeddingWorkerPollIntervalSeconds:   getEnvInt("BACKGROUND_EMBEDDING_WORKER_POLL_INTERVAL_SECONDS", 2),
		},
	}
	return cfg, cfg.Validate()
}

func (c *Config) Validate() error {
	if c.App.Env == "production" {
		if len(c.JWT.Secret) < 32 || strings.HasPrefix(c.JWT.Secret, "dev-secret") {
			return fmt.Errorf("production requires a strong JWT_SECRET")
		}
		if c.Database.Password == "" || c.Database.Password == "postgres" {
			return fmt.Errorf("production requires a non-default PG_PASS")
		}
	}
	switch c.Admission.PolicyType {
	case "protocol", "invitation", "approval":
	default:
		return fmt.Errorf("unsupported ADMISSION_POLICY %q", c.Admission.PolicyType)
	}
	if strings.TrimSpace(c.ObjectStorage.Endpoint) == "" {
		return fmt.Errorf("OBJECT_STORAGE_ENDPOINT is required")
	}
	if strings.TrimSpace(c.ObjectStorage.PublicEndpoint) == "" {
		return fmt.Errorf("OBJECT_STORAGE_PUBLIC_ENDPOINT is required")
	}
	if strings.TrimSpace(c.ObjectStorage.Bucket) == "" {
		return fmt.Errorf("OBJECT_STORAGE_BUCKET is required")
	}
	if c.ObjectStorage.UploadTTLSecs <= 0 {
		return fmt.Errorf("OBJECT_UPLOAD_PRESIGN_TTL_SECONDS must be positive")
	}
	if c.ObjectStorage.DownloadTTLSecs <= 0 {
		return fmt.Errorf("OBJECT_DOWNLOAD_PRESIGN_TTL_SECONDS must be positive")
	}
	switch c.Embedding.Provider {
	case "disabled", "deterministic", "local_http":
	default:
		return fmt.Errorf("unsupported EMBEDDING_PROVIDER %q", c.Embedding.Provider)
	}
	if c.Embedding.Dimensions <= 0 {
		return fmt.Errorf("EMBEDDING_DIMENSIONS must be positive")
	}
	if c.Embedding.TimeoutSecs <= 0 {
		return fmt.Errorf("EMBEDDING_TIMEOUT_SECONDS must be positive")
	}
	if c.Embedding.MaxBatchSize <= 0 {
		return fmt.Errorf("EMBEDDING_MAX_BATCH_SIZE must be positive")
	}
	if c.Embedding.Provider == "local_http" && strings.TrimSpace(c.Embedding.Endpoint) == "" {
		return fmt.Errorf("EMBEDDING_ENDPOINT is required when EMBEDDING_PROVIDER=local_http")
	}
	if c.Background.OfflineMessageCleanupIntervalSeconds <= 0 {
		return fmt.Errorf("BACKGROUND_OFFLINE_CLEANUP_INTERVAL_SECONDS must be positive")
	}
	if c.Background.SyncEventCleanupIntervalSeconds <= 0 {
		return fmt.Errorf("BACKGROUND_SYNC_CLEANUP_INTERVAL_SECONDS must be positive")
	}
	if c.Background.SensitiveConfirmationIntervalSeconds <= 0 {
		return fmt.Errorf("BACKGROUND_SENSITIVE_CONFIRMATION_INTERVAL_SECONDS must be positive")
	}
	if c.Background.KBSubscriptionExpiryIntervalSeconds <= 0 {
		return fmt.Errorf("BACKGROUND_KB_SUBSCRIPTION_EXPIRY_INTERVAL_SECONDS must be positive")
	}
	if c.Background.EmbeddingWorkerBatchSize <= 0 {
		return fmt.Errorf("BACKGROUND_EMBEDDING_WORKER_BATCH_SIZE must be positive")
	}
	if c.Background.EmbeddingWorkerPollIntervalSeconds <= 0 {
		return fmt.Errorf("BACKGROUND_EMBEDDING_WORKER_POLL_INTERVAL_SECONDS must be positive")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func getEnvInt(key string, fallback int) int {
	v, err := strconv.Atoi(getEnv(key, ""))
	if err != nil {
		return fallback
	}
	return v
}
func getEnvBool(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(getEnv(key, "")))
	if v == "" {
		return fallback
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}
func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}
