package storage

// Config holds configuration options.
type Config struct {
	AccessKeyId     string
	SecretAccessKey string
	Bucket          string
	Endpoint        string
	Region          string
	UsePathStyle    bool
	Provider        string
}

// DefaultConfig returns the default config.
func DefaultConfig() *Config {
	return &Config{
		AccessKeyId:     "",
		SecretAccessKey: "",
		Bucket:          "",
		Endpoint:        "http://localhost:9000",
		Region:          "us-east-1",
		UsePathStyle:    false,
	}
}
