package station

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexwitherspoon/aviationwx-bridge/internal/config"
)

func TestEcowittPollFixture(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "ecowitt_get_livedata_info.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ecowittLivedataPath {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	e := NewEcowitt()
	e.client = srv.Client()
	obs, err := e.Poll(context.Background(), config.Station{
		ID:   "station-ecowitt",
		Type: config.StationTypeEcowittGateway,
		Host: srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if obs.Provider != ProviderEcowittGateway {
		t.Fatalf("provider = %q", obs.Provider)
	}
	if obs.ObservedAt.IsZero() {
		t.Fatal("expected ObservedAt from 0x18")
	}
	want := time.Date(2023, 2, 3, 14, 22, 11, 0, time.UTC)
	if !obs.ObservedAt.Equal(want) {
		t.Fatalf("ObservedAt = %v, want %v", obs.ObservedAt, want)
	}
	raw, ok := obs.ProviderMeta["raw"].(map[string]interface{})
	if !ok || raw["common_list"] == nil {
		t.Fatalf("provider_meta.raw = %#v", obs.ProviderMeta["raw"])
	}
	if obs.ProviderMeta["api"] != ecowittAPIMeta {
		t.Fatalf("api = %v", obs.ProviderMeta["api"])
	}
}

func TestEcowittPollMissingTimeSkipsObservedAt(t *testing.T) {
	body := []byte(`{"common_list":[{"id":"0x02","val":"20.0","unit":"C"}],"wh25":[{"intemp":"21.0","unit":"C","inhumi":"50%","abs":"1000 hPa","rel":"1000 hPa"}]}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	e := NewEcowitt()
	e.client = srv.Client()
	obs, err := e.Poll(context.Background(), config.Station{
		ID: "s1", Type: config.StationTypeEcowittGateway, Host: srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !obs.ObservedAt.IsZero() {
		t.Fatalf("ObservedAt = %v, want zero without 0x18", obs.ObservedAt)
	}
}

func TestEcowittPollRejectsNonLivedata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":"Version: GW2000B_V2.1.4","platform":"ecowitt"}`))
	}))
	defer srv.Close()
	e := NewEcowitt()
	e.client = srv.Client()
	if _, err := e.Poll(context.Background(), config.Station{Host: srv.URL}); err == nil {
		t.Fatal("expected error for get_version-shaped body")
	}
}

func TestParseEcowittTimeVal(t *testing.T) {
	tests := []struct {
		name string
		val  string
		want time.Time
		ok   bool
	}{
		{
			name: "gateway space format assumed UTC",
			val:  "2024-11-25 20:41:16",
			want: time.Date(2024, 11, 25, 20, 41, 16, 0, time.UTC),
			ok:   true,
		},
		{
			name: "ISO T format assumed UTC",
			val:  "2024-11-25T20:41:16",
			want: time.Date(2024, 11, 25, 20, 41, 16, 0, time.UTC),
			ok:   true,
		},
		{
			name: "compact form assumed UTC",
			val:  "20240517125938",
			want: time.Date(2024, 5, 17, 12, 59, 38, 0, time.UTC),
			ok:   true,
		},
		{name: "empty", val: "", ok: false},
		{name: "garbage", val: "not-a-time", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseEcowittTimeVal(tt.val)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if tt.ok && !got.Equal(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEcowittURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
		err  bool
	}{
		{"192.168.1.60", "http://192.168.1.60:80/get_livedata_info", false},
		{"192.168.1.60:8080", "http://192.168.1.60:8080/get_livedata_info", false},
		{"http://example.local:8080/ignored", "http://example.local:8080/get_livedata_info", false},
		{"https://gw.local", "https://gw.local/get_livedata_info", false},
		{"2001:db8::1", "http://[2001:db8::1]:80/get_livedata_info", false},
		{"[2001:db8::1]", "http://[2001:db8::1]:80/get_livedata_info", false},
		{"[2001:db8::1]:8080", "http://[2001:db8::1]:8080/get_livedata_info", false},
		{"", "", true},
		{"ftp://gw.local", "", true},
		{"192.168.1.60/other", "", true},
	}
	for _, tc := range cases {
		u, err := ecowittURL(tc.in, ecowittLivedataPath)
		if tc.err {
			if err == nil {
				t.Fatalf("%q: expected error, got %s", tc.in, u)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if u != tc.want {
			t.Fatalf("%q: got %s want %s", tc.in, u, tc.want)
		}
	}
}
