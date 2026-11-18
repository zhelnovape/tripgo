package config

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTP            HTTP
	LogLevel        string
	ShutdownTimeout time.Duration
	DB              Database
	Telemetry       Telemetry
	Positions       Positions
	Push            Push
}

type Positions struct {
	CheckInterval time.Duration
	StaleAfter    time.Duration
	Workers       int
	QueueSize     int
}

type Push struct {
	HTTPURL        string
	RequestTimeout time.Duration
}

type HTTP struct {
	Addr              string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

type Database struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	ConnectTimeout  time.Duration
	QueryTimeout    time.Duration
}

type Telemetry struct {
	Endpoint string
	Protocol string
}

func Load() (*Config, error) {
	var err error
	cfg := &Config{}

	if cfg.HTTP.Addr, err = getStrOrErr("HTTP_ADDR"); err != nil {
		return nil, err
	}
	if cfg.HTTP.ReadTimeout, err = getDurOrErr("HTTP_READ_TIMEOUT"); err != nil {
		return nil, err
	}
	if cfg.HTTP.ReadHeaderTimeout, err = getDurOrErr("HTTP_READ_HEADER_TIMEOUT"); err != nil {
		return nil, err
	}
	if cfg.HTTP.WriteTimeout, err = getDurOrErr("HTTP_WRITE_TIMEOUT"); err != nil {
		return nil, err
	}
	if cfg.HTTP.IdleTimeout, err = getDurOrErr("HTTP_IDLE_TIMEOUT"); err != nil {
		return nil, err
	}

	if cfg.LogLevel, err = getStrOrErr("LOG_LEVEL"); err != nil {
		return nil, err
	}
	if cfg.ShutdownTimeout, err = getDurOrErr("SHUTDOWN_TIMEOUT"); err != nil {
		return nil, err
	}

	if cfg.DB.URL, err = getStrOrErr("DATABASE_URL"); err != nil {
		return nil, err
	}

	if cfg.DB.MaxConns, err = getInt32OrErr("DATABASE_MAX_CONNS"); err != nil {
		return nil, err
	}
	if cfg.DB.MaxConns <= 0 {
		return nil, fmt.Errorf("DATABASE_MAX_CONNS must be positive, got %d", cfg.DB.MaxConns)
	}
	if cfg.DB.MinConns, err = getInt32OrErr("DATABASE_MIN_CONNS"); err != nil {
		return nil, err
	}
	if cfg.DB.MinConns > cfg.DB.MaxConns {
		return nil, fmt.Errorf("DATABASE_MIN_CONNS (%d) must be <= DATABASE_MAX_CONNS (%d)",
			cfg.DB.MinConns, cfg.DB.MaxConns)
	}

	if cfg.DB.MaxConnLifetime, err = getDurOrErr("DATABASE_MAX_CONN_LIFETIME"); err != nil {
		return nil, err
	}
	if cfg.DB.ConnectTimeout, err = getDurOrErr("DATABASE_CONNECT_TIMEOUT"); err != nil {
		return nil, err
	}
	if cfg.DB.QueryTimeout, err = getDurOrErr("DATABASE_QUERY_TIMEOUT"); err != nil {
		return nil, err
	}

	if cfg.Telemetry.Endpoint, err = getStrOrErr("OTEL_EXPORTER_OTLP_ENDPOINT"); err != nil {
		return nil, err
	}
	if cfg.Telemetry.Protocol, err = getStrOrErr("OTEL_EXPORTER_OTLP_PROTOCOL"); err != nil {
		return nil, err
	}
	if cfg.Telemetry.Protocol != "http/protobuf" && cfg.Telemetry.Protocol != "grpc" {
		return nil, fmt.Errorf("OTEL_EXPORTER_OTLP_PROTOCOL must be http/protobuf or grpc, got %q", cfg.Telemetry.Protocol)
	}

	if err := loadPositions(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadPositions(cfg *Config) error {
	var err error

	if cfg.Positions.CheckInterval, err = getDurOrDefault("POSITION_CHECK_INTERVAL", 30*time.Second); err != nil {
		return err
	}
	if cfg.Positions.StaleAfter, err = getDurOrDefault("POSITION_STALE_AFTER", 60*time.Second); err != nil {
		return err
	}
	if cfg.Positions.Workers, err = getPositiveIntOrDefault("POSITION_WORKERS", 4); err != nil {
		return err
	}
	if cfg.Positions.QueueSize, err = getPositiveIntOrDefault("POSITION_QUEUE_SIZE", 100); err != nil {
		return err
	}
	if cfg.Push.HTTPURL, err = getURLOrErr("PUSH_HTTP_URL"); err != nil {
		return err
	}
	if cfg.Push.RequestTimeout, err = getDurOrDefault("PUSH_REQUEST_TIMEOUT", time.Second); err != nil {
		return err
	}
	return nil
}

func getURLOrErr(varEnv string) (string, error) {
	val, err := getStrOrErr(varEnv)
	if err != nil {
		return "", err
	}

	u, err := url.Parse(val)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%s must be an absolute http(s) URL, got %q", varEnv, val)
	}
	return val, nil
}

func getDurOrDefault(varEnv string, def time.Duration) (time.Duration, error) {
	if os.Getenv(varEnv) == "" {
		return def, nil
	}
	return getDurOrErr(varEnv)
}

func getPositiveIntOrDefault(varEnv string, def int) (int, error) {
	if os.Getenv(varEnv) == "" {
		return def, nil
	}

	num, err := getInt32OrErr(varEnv)
	if err != nil {
		return 0, err
	}
	if num == 0 {
		return 0, fmt.Errorf("%s must be positive, got %d", varEnv, num)
	}
	return int(num), nil
}

func getStrOrErr(varEnv string) (string, error) {
	if val := os.Getenv(varEnv); val != "" {
		return val, nil
	}
	return "", fmt.Errorf("environment variable %s is required", varEnv)
}

func getDurOrErr(varEnv string) (time.Duration, error) {
	val, err := getStrOrErr(varEnv)
	if err != nil {
		return 0, err
	}

	d, err := time.ParseDuration(val)
	if err != nil {
		return 0, fmt.Errorf("invalid duration for %s: %w", varEnv, err)
	}

	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive, got %s", varEnv, d)
	}

	return d, nil
}

func getInt32OrErr(varEnv string) (int32, error) {
	val, err := getStrOrErr(varEnv)
	if err != nil {
		return 0, err
	}

	num, err := strconv.ParseInt(val, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid value for %s: %w", varEnv, err)
	}

	if num < 0 || num > math.MaxInt32 {
		return 0, fmt.Errorf("%s must be between 0 and %d, got %d", varEnv, math.MaxInt32, num)
	}

	return int32(num), nil
}
