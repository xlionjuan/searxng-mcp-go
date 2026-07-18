package searxng

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()

	if cfg.AllowGETFallback {
		t.Fatal("AllowGETFallback = true, want false by default")
	}
}

// --- Config.Validate tests ---

//nolint:gocognit // validates many config fields across parallel subtests
func TestConfigValidate(t *testing.T) {
	t.Parallel()

	t.Run("valid config", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			SearXNGURL:       "https://search.example.com",
			Timeout:          5 * time.Second,
			MaxRetries:       3,
			RetryDelay:       1 * time.Second,
			MaxRetryDelay:    30 * time.Second,
			AllowGETFallback: true,
		}

		err := cfg.Validate()
		if err != nil {
			t.Fatalf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("empty URL", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{Timeout: time.Second}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("Validate() error = nil, want error")
		}
	})

	t.Run("negative timeout", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{SearXNGURL: "https://example.com", Timeout: -1}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("Validate() error = nil, want error")
		}
	})

	t.Run("negative MaxRetries", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{SearXNGURL: "https://example.com", MaxRetries: -1}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("Validate() error = nil, want error")
		}
	})

	t.Run("negative RetryDelay", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{SearXNGURL: "https://example.com", RetryDelay: -1}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("Validate() error = nil, want error")
		}
	})

	t.Run("negative MaxRetryDelay", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{SearXNGURL: "https://example.com", MaxRetryDelay: -1}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("Validate() error = nil, want error")
		}
	})

	t.Run("sub-second RetryDelay", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{SearXNGURL: "https://example.com", RetryDelay: 500 * time.Millisecond}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("Validate() error = nil, want error for sub-second RetryDelay")
		}
	})

	t.Run("sub-second MaxRetryDelay", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{SearXNGURL: "https://example.com", MaxRetryDelay: 999 * time.Millisecond}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("Validate() error = nil, want error for sub-second MaxRetryDelay")
		}
	})

	t.Run("zero RetryDelay accepted by Validate", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{SearXNGURL: "https://example.com", RetryDelay: 0}

		err := cfg.Validate()
		if err != nil {
			t.Fatalf("Validate() with zero RetryDelay = %v, want nil (normalized later)", err)
		}
	})

	t.Run("zero MaxRetryDelay accepted by Validate", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{SearXNGURL: "https://example.com", MaxRetryDelay: 0}

		err := cfg.Validate()
		if err != nil {
			t.Fatalf("Validate() with zero MaxRetryDelay = %v, want nil (normalized later)", err)
		}
	})

	t.Run("zero Timeout accepted by Validate (struct literal path)", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{SearXNGURL: "https://example.com", Timeout: 0}

		err := cfg.Validate()
		if err != nil {
			t.Fatalf("Validate() with zero Timeout = %v, want nil (normalized later)", err)
		}
	})

	t.Run("second-exact RetryDelay accepted", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{SearXNGURL: "https://example.com", RetryDelay: time.Second}

		err := cfg.Validate()
		if err != nil {
			t.Fatalf("Validate() with 1s RetryDelay = %v, want nil", err)
		}
	})
}

// --- Config.SetTimeout tests ---

func TestConfigSetTimeout(t *testing.T) {
	t.Parallel()

	t.Run("negative rejected", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{SearXNGURL: "https://example.com"}

		err := cfg.SetTimeout(-1)
		if err == nil {
			t.Fatal("SetTimeout(-1) error = nil, want error")
		}
	})

	t.Run("zero rejected via SetTimeout (CLI/env path)", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{SearXNGURL: "https://example.com"}

		err := cfg.SetTimeout(0)
		if err == nil {
			t.Fatal("SetTimeout(0) error = nil, want error")
		}
	})

	t.Run("positive accepted", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{SearXNGURL: "https://example.com"}

		err := cfg.SetTimeout(30 * time.Second)
		if err != nil {
			t.Fatalf("SetTimeout(30s) error = %v, want nil", err)
		}

		if cfg.Timeout != 30*time.Second {
			t.Fatalf("Timeout = %v, want 30s", cfg.Timeout)
		}
	})
}

// --- Config.Normalize tests ---

//nolint:gocognit // validates many normalize cases across parallel subtests
func TestConfigNormalize(t *testing.T) {
	t.Parallel()

	t.Run("preserves valid config", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			SearXNGURL:       "https://search.example.com",
			Timeout:          5 * time.Second,
			MaxRetries:       3,
			RetryDelay:       1 * time.Second,
			MaxRetryDelay:    30 * time.Second,
			AllowGETFallback: true,
		}

		normalized := cfg.Normalize()

		if normalized.MaxRetries != 3 {
			t.Fatalf("MaxRetries = %d, want 3", normalized.MaxRetries)
		}

		if normalized.RetryDelay != time.Second {
			t.Fatalf("RetryDelay = %v, want 1s", normalized.RetryDelay)
		}

		if normalized.MaxRetryDelay != 30*time.Second {
			t.Fatalf("MaxRetryDelay = %v, want 30s", normalized.MaxRetryDelay)
		}

		if !normalized.AllowGETFallback {
			t.Fatal("AllowGETFallback = false, want true")
		}
	})

	t.Run("defaults zero RetryDelay", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{RetryDelay: 0}
		normalized := cfg.Normalize()

		if normalized.RetryDelay != DefaultRetryDelay {
			t.Fatalf("RetryDelay = %v, want %v", normalized.RetryDelay, DefaultRetryDelay)
		}
	})

	t.Run("defaults zero MaxRetryDelay", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{MaxRetryDelay: 0}
		normalized := cfg.Normalize()

		if normalized.MaxRetryDelay != DefaultMaxRetryDelay {
			t.Fatalf("MaxRetryDelay = %v, want %v", normalized.MaxRetryDelay, DefaultMaxRetryDelay)
		}
	})

	t.Run("clamps MaxRetryDelay to at least RetryDelay", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{RetryDelay: 5 * time.Second, MaxRetryDelay: 1 * time.Second}
		normalized := cfg.Normalize()

		if normalized.MaxRetryDelay != 5*time.Second {
			t.Fatalf("MaxRetryDelay = %v, want %v (clamped to RetryDelay)", normalized.MaxRetryDelay, 5*time.Second)
		}
	})

	t.Run("normalizes zero Timeout to DefaultTimeout", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{Timeout: 0}
		normalized := cfg.Normalize()

		if normalized.Timeout != DefaultTimeout {
			t.Fatalf("Timeout = %v, want %v", normalized.Timeout, DefaultTimeout)
		}
	})

	t.Run("preserves positive Timeout", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{Timeout: 30 * time.Second}
		normalized := cfg.Normalize()

		if normalized.Timeout != 30*time.Second {
			t.Fatalf("Timeout = %v, want 30s", normalized.Timeout)
		}
	})

	t.Run("does not normalize negative Timeout (Validate must catch)", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{Timeout: -1}
		normalized := cfg.Normalize()

		if normalized.Timeout != -1 {
			t.Fatalf("Timeout = %v, want -1 (Normalize should not touch negative)", normalized.Timeout)
		}
	})

	t.Run("does not normalize negative RetryDelay (Validate must catch)", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{RetryDelay: -1}
		normalized := cfg.Normalize()

		if normalized.RetryDelay != -1 {
			t.Fatalf("RetryDelay = %v, want -1 (Normalize should not touch negative)", normalized.RetryDelay)
		}
	})
}

// --- UnescapeIfNeeded tests ---

func TestUnescapeIfNeeded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain string unchanged", input: "hello world", want: "hello world"},
		{name: "ampersand unescaped", input: "foo &amp; bar", want: "foo & bar"},
		{name: "lt unescaped", input: "a &lt; b", want: "a < b"},
		{name: "gt unescaped", input: "a &gt; b", want: "a > b"},
		{name: "quote unescaped", input: "&quot;quoted&quot;", want: "\"quoted\""},
		{name: "numeric entity", input: "&#x26;", want: "&"},
		{name: "no entities but has angle bracket", input: "a < b", want: "a < b"},
		{name: "empty string", input: "", want: ""},
		{name: "only entity characters in content", input: "a&b", want: "a&b"},
		{name: "double quote in string", input: `he said "hello"`, want: `he said "hello"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := UnescapeIfNeeded(tt.input); got != tt.want {
				t.Fatalf("UnescapeIfNeeded(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- WeatherMeasure.String tests ---

func TestWeatherMeasureString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		val  float64
		unit string
		want string
	}{
		{name: "zero value", val: 0, unit: "", want: ""},
		{name: "value only", val: 42.5, unit: "", want: "42.5"},
		{name: "value with unit", val: 11.2, unit: "°C", want: "11.2 °C"},
		{name: "unit only", val: 0, unit: "m/s", want: "0 m/s"},
		{name: "integer value", val: 100, unit: "hPa", want: "100 hPa"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := WeatherMeasure{Val: tt.val, Unit: tt.unit}

			if got := m.String(); got != tt.want {
				t.Fatalf("WeatherMeasure{%v, %q}.String() = %q, want %q", tt.val, tt.unit, got, tt.want)
			}
		})
	}
}
