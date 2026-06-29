package service

import (
	"strings"
	"testing"
)

// CE-M1-9 §6 单测覆盖:
//   - BuildFilename 拼装规则(全字段 / 缺省字段 / 错误输入)
//   - ParseFilenameOracle 是 rustdesk/src/custom_server.rs:39 的 Go 端等价实现,
//     用于验证"生成的文件名能被客户端解析"

func TestBuildFilename_AllFields(t *testing.T) {
	host := "id.example.com:21116"
	key := "OeVuKk5nlHiXp+APNn0Y3pC1Iwpwn44JGqrQCsWqmBw="
	api := "https://api.example.com"
	relay := "relay.example.com:21117"
	got, err := BuildFilename(host, key, api, relay)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.HasPrefix(got, "EstelRemote-host=") {
		t.Fatalf("missing prefix: %q", got)
	}
	if !strings.HasSuffix(got, ".exe") {
		t.Fatalf("missing .exe suffix: %q", got)
	}
	// 段内不允许出现裸 `,`(必须 percent-encode)
	// 因此 strings.Count(got, ",") 应等于 3(host/key/api/relay 之间的 3 个分隔逗号)
	if c := strings.Count(got, ","); c != 3 {
		t.Fatalf("expected 3 commas as segment separators, got %d: %q", c, got)
	}
}

func TestBuildFilename_RustDeskClientParse(t *testing.T) {
	host := "id.example.com:21116"
	key := "OeVuKk5nlHiXp+APNn0Y3pC1Iwpwn44JGqrQCsWqmBw="
	api := "https://api.example.com"
	relay := "relay.example.com:21117"
	got, err := BuildFilename(host, key, api, relay)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	h2, k2, a2, r2, ok := ParseFilenameOracle(got)
	if !ok {
		t.Fatalf("oracle failed to parse %q", got)
	}
	if h2 != host || k2 != key || a2 != api || r2 != relay {
		t.Fatalf("round-trip mismatch:\n got host=%q key=%q api=%q relay=%q\nwant host=%q key=%q api=%q relay=%q",
			h2, k2, a2, r2, host, key, api, relay)
	}
}

func TestBuildFilename_OmitEmpty(t *testing.T) {
	got, err := BuildFilename("id.example.com", "", "", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "EstelRemote-host=id.example.com.exe"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if strings.Contains(got, "key=") || strings.Contains(got, "api=") || strings.Contains(got, "relay=") {
		t.Fatalf("empty fields should be omitted, got %q", got)
	}
}

func TestBuildFilename_RejectEmptyHost(t *testing.T) {
	_, err := BuildFilename("   ", "k", "a", "r")
	if err == nil {
		t.Fatalf("expected error for empty host")
	}
}

func TestBuildFilename_BackwardCompatNoRelay(t *testing.T) {
	// CE-M1-9 §6 测试 #10:老前端不传 relay_server 字段。
	got, err := BuildFilename("id.example.com", "k=", "https://api.example.com", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if strings.Contains(got, "relay=") {
		t.Fatalf("relay segment should be omitted, got %q", got)
	}
	_, k, a, r, ok := ParseFilenameOracle(got)
	if !ok || k != "k=" || a != "https://api.example.com" || r != "" {
		t.Fatalf("parse mismatch got k=%q a=%q r=%q", k, a, r)
	}
}

func TestParseFilenameOracle_UpstreamFixtures(t *testing.T) {
	// 与 rustdesk/src/custom_server.rs:119-184 的测试用例对齐,
	// 验证 oracle 兼容大小写不敏感前缀与 .exe.exe 后缀。
	type tc struct {
		in                  string
		host, key, api, rel string
	}
	cases := []tc{
		{"rustdesk-host=server.example.net.exe", "server.example.net", "", "", ""},
		{"rustdesk-host=server.example.net,api=abc,key=Zm9vYmFyLiwyCg==.exe",
			"server.example.net", "Zm9vYmFyLiwyCg==", "abc", ""},
		{"rustdesk-host=server.example.net,key=Zm9vYmFyLiwyCg==,relay=server.example.net.exe",
			"server.example.net", "Zm9vYmFyLiwyCg==", "", "server.example.net"},
		{"rustdesk-Host=server.example.net,Key=Zm9vYmFyLiwyCg==,RELAY=server.example.net.exe",
			"server.example.net", "Zm9vYmFyLiwyCg==", "", "server.example.net"},
	}
	for _, c := range cases {
		h, k, a, r, ok := ParseFilenameOracle(c.in)
		if !ok {
			t.Fatalf("parse failed: %q", c.in)
		}
		if h != c.host || k != c.key || a != c.api || r != c.rel {
			t.Fatalf("mismatch for %q: got %q,%q,%q,%q want %q,%q,%q,%q",
				c.in, h, k, a, r, c.host, c.key, c.api, c.rel)
		}
	}
}

func TestSafeFieldEncode_EscapesCommaAndSpaces(t *testing.T) {
	got := safeFieldEncode("a,b c\td")
	if strings.ContainsAny(got, ", \t") {
		t.Fatalf("unsafe chars remain: %q", got)
	}
}
