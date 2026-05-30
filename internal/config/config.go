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
