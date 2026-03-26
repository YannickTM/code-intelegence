package extractors

import (
	"context"
	"testing"

	"myjungle/backend-worker/internal/parser"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func parseAndExtractNetwork(t *testing.T, langID, source string) []parser.NetworkCall {
	t.Helper()
	grammar := parser.GetGrammar(langID)
	if grammar == nil {
		t.Fatalf("no grammar for %s", langID)
	}
	pool := parser.NewPool(1)
	defer pool.Shutdown()

	content := []byte(source)
	tree, err := pool.Parse(context.Background(), content, grammar)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	root := tree.RootNode()
	symbols := ExtractSymbols(root, content, langID)
	return ExtractNetworkCalls(root, content, symbols, langID)
}

func findNetworkCall(calls []parser.NetworkCall, clientKind string) *parser.NetworkCall {
	for i := range calls {
		if calls[i].ClientKind == clientKind {
			return &calls[i]
		}
	}
	return nil
}

func findNetworkCallByMethod(calls []parser.NetworkCall, method string) *parser.NetworkCall {
	for i := range calls {
		if calls[i].Method == method {
			return &calls[i]
		}
	}
	return nil
}

func assertNetworkCallExists(t *testing.T, calls []parser.NetworkCall, clientKind string) *parser.NetworkCall {
	t.Helper()
	c := findNetworkCall(calls, clientKind)
	if c == nil {
		kinds := make([]string, len(calls))
		for i, c := range calls {
			kinds[i] = c.ClientKind + ":" + c.Method
		}
		t.Errorf("expected network call with kind %q not found; have: %v", clientKind, kinds)
	}
	return c
}

// ---------------------------------------------------------------------------
// JS/TS fetch tests
// ---------------------------------------------------------------------------

func TestJSFetchDetection(t *testing.T) {
	calls := parseAndExtractNetwork(t, "typescript", `
async function loadData() {
  const res = await fetch("/api/users");
}
`)
	c := assertNetworkCallExists(t, calls, "FETCH")
	if c != nil {
		if c.Method != "GET" {
			t.Errorf("expected GET, got %s", c.Method)
		}
		if c.URLLiteral != "/api/users" {
			t.Errorf("expected /api/users, got %s", c.URLLiteral)
		}
		if !c.IsRelative {
			t.Error("expected IsRelative = true")
		}
	}
}

func TestJSFetchWithMethod(t *testing.T) {
	calls := parseAndExtractNetwork(t, "typescript", `
async function createUser() {
  const res = await fetch("/api/users", { method: "POST" });
}
`)
	c := assertNetworkCallExists(t, calls, "FETCH")
	if c != nil && c.Method != "POST" {
		t.Errorf("expected POST, got %s", c.Method)
	}
}

func TestJSFetchAbsoluteURL(t *testing.T) {
	calls := parseAndExtractNetwork(t, "typescript", `
fetch("https://api.example.com/users");
`)
	c := assertNetworkCallExists(t, calls, "FETCH")
	if c != nil {
		if c.IsRelative {
			t.Error("expected IsRelative = false for https URL")
		}
		if c.URLLiteral != "https://api.example.com/users" {
			t.Errorf("unexpected URL: %s", c.URLLiteral)
		}
	}
}

// ---------------------------------------------------------------------------
// Axios tests
// ---------------------------------------------------------------------------

func TestAxiosGet(t *testing.T) {
	calls := parseAndExtractNetwork(t, "typescript", `
const data = await axios.get("/api/users");
`)
	c := assertNetworkCallExists(t, calls, "AXIOS")
	if c != nil {
		if c.Method != "GET" {
			t.Errorf("expected GET, got %s", c.Method)
		}
	}
}

func TestAxiosPost(t *testing.T) {
	calls := parseAndExtractNetwork(t, "typescript", `
await axios.post("/api/users", { name: "John" });
`)
	c := assertNetworkCallExists(t, calls, "AXIOS")
	if c != nil {
		if c.Method != "POST" {
			t.Errorf("expected POST, got %s", c.Method)
		}
	}
}

func TestAxiosDirect(t *testing.T) {
	calls := parseAndExtractNetwork(t, "typescript", `
const res = await axios("/api/users");
`)
	c := assertNetworkCallExists(t, calls, "AXIOS")
	if c != nil {
		if c.URLLiteral != "/api/users" {
			t.Errorf("expected /api/users, got %s", c.URLLiteral)
		}
	}
}

// ---------------------------------------------------------------------------
// Ky tests
// ---------------------------------------------------------------------------

func TestKyGet(t *testing.T) {
	calls := parseAndExtractNetwork(t, "typescript", `
const data = await ky.get("/api/users").json();
`)
	c := assertNetworkCallExists(t, calls, "KY")
	if c != nil && c.Method != "GET" {
		t.Errorf("expected GET, got %s", c.Method)
	}
}

// ---------------------------------------------------------------------------
// GraphQL tests
// ---------------------------------------------------------------------------

func TestGraphQLUseQuery(t *testing.T) {
	calls := parseAndExtractNetwork(t, "tsx", `
function Users() {
  const { data } = useQuery(GET_USERS);
  return <div>{data}</div>;
}
`)
	c := assertNetworkCallExists(t, calls, "GRAPHQL")
	if c != nil && c.Method != "GET" {
		t.Errorf("expected GET for useQuery, got %s", c.Method)
	}
}

func TestGraphQLUseMutation(t *testing.T) {
	calls := parseAndExtractNetwork(t, "typescript", `
const [createUser] = useMutation(CREATE_USER);
`)
	c := assertNetworkCallExists(t, calls, "GRAPHQL")
	if c != nil && c.Method != "POST" {
		t.Errorf("expected POST for useMutation, got %s", c.Method)
	}
}

// ---------------------------------------------------------------------------
// Instance pattern tests
// ---------------------------------------------------------------------------

func TestInstancePattern(t *testing.T) {
	calls := parseAndExtractNetwork(t, "typescript", `
const api = createAPI();
const data = await api.get("/users");
`)
	if len(calls) == 0 {
		t.Fatal("expected network call for api.get()")
	}
	c := calls[0]
	if c.Method != "GET" {
		t.Errorf("expected GET, got %s", c.Method)
	}
	if c.Confidence != "MEDIUM" {
		t.Errorf("expected MEDIUM confidence for instance pattern, got %s", c.Confidence)
	}
}

func TestInstancePatternNotGraphQL(t *testing.T) {
	// db.query() should NOT be classified as GraphQL.
	// It may or may not be detected as a network call, but if detected
	// it must not be GraphQL.
	calls := parseAndExtractNetwork(t, "typescript", `
const result = await db.query("SELECT * FROM users");
`)
	for _, c := range calls {
		if c.ClientKind == "GRAPHQL" {
			t.Errorf("db.query() should not be classified as GRAPHQL")
		}
	}
}

func TestGraphQLReceiverGating(t *testing.T) {
	// apolloClient.query() should be classified as GraphQL.
	calls := parseAndExtractNetwork(t, "typescript", `
const { data } = await apolloClient.query({ query: GET_USERS });
`)
	c := assertNetworkCallExists(t, calls, "GRAPHQL")
	if c != nil {
		if c.Method != "POST" {
			t.Errorf("expected POST, got %s", c.Method)
		}
		if c.Confidence != "HIGH" {
			t.Errorf("expected HIGH confidence for known GraphQL client, got %s", c.Confidence)
		}
	}
}

func TestGraphQLReceiverRequest(t *testing.T) {
	// graphqlClient.request() should be GraphQL, but http.request() should not.
	calls := parseAndExtractNetwork(t, "typescript", `
const data = await graphqlClient.request(query);
`)
	c := assertNetworkCallExists(t, calls, "GRAPHQL")
	if c == nil {
		t.Fatal("expected graphqlClient.request() to be classified as GRAPHQL")
	}

	// someApi.request() should not be classified as GraphQL.
	// It may or may not be detected, but if detected it must not be GraphQL.
	calls2 := parseAndExtractNetwork(t, "typescript", `
const res = await someApi.request("/endpoint");
`)
	for _, c := range calls2 {
		if c.ClientKind == "GRAPHQL" {
			t.Errorf("someApi.request() should not be classified as GRAPHQL")
		}
	}
}

func TestFetchHighConfidence(t *testing.T) {
	calls := parseAndExtractNetwork(t, "typescript", `
const res = await fetch("/api/data");
`)
	if len(calls) == 0 {
		t.Fatal("expected network call")
	}
	if calls[0].Confidence != "HIGH" {
		t.Errorf("expected HIGH confidence for fetch(), got %s", calls[0].Confidence)
	}
}

// ---------------------------------------------------------------------------
// Template literal URL tests
// ---------------------------------------------------------------------------

func TestTemplateLiteralURL(t *testing.T) {
	calls := parseAndExtractNetwork(t, "typescript", "const res = await fetch(`/api/users/${id}`);")
	if len(calls) == 0 {
		t.Fatal("expected network call")
	}
	c := calls[0]
	if c.URLTemplate != "" {
		// Template URL should contain the substitution pattern.
		if c.URLTemplate != "/api/users/{expr}" {
			t.Errorf("expected URLTemplate /api/users/{expr}, got %s", c.URLTemplate)
		}
	} else if c.URLLiteral != "<dynamic>" {
		t.Errorf("expected URLLiteral <dynamic> as fallback, got %s", c.URLLiteral)
	}
}

func TestDynamicURL(t *testing.T) {
	calls := parseAndExtractNetwork(t, "typescript", `
const url = getURL();
fetch(url);
`)
	if len(calls) == 0 {
		t.Fatal("expected network call")
	}
	c := calls[0]
	if c.URLLiteral != "<dynamic>" {
		t.Errorf("expected <dynamic>, got %s", c.URLLiteral)
	}
}

// ---------------------------------------------------------------------------
// Enclosing symbol tests
// ---------------------------------------------------------------------------

func TestNetworkEnclosingSymbol(t *testing.T) {
	calls := parseAndExtractNetwork(t, "typescript", `
async function loadData() {
  const res = await fetch("/api/data");
}
`)
	if len(calls) == 0 {
		t.Fatal("expected network call")
	}
	if calls[0].SourceSymbolID == "" {
		t.Error("expected SourceSymbolID for enclosing function")
	}
}

// ---------------------------------------------------------------------------
// Python HTTP client tests
// ---------------------------------------------------------------------------

func TestPythonRequests(t *testing.T) {
	calls := parseAndExtractNetwork(t, "python", `
import requests

response = requests.get("http://api.example.com/users")
`)
	if len(calls) == 0 {
		t.Fatal("expected network call for requests.get()")
	}
	c := calls[0]
	if c.Method != "GET" {
		t.Errorf("expected GET, got %s", c.Method)
	}
}

func TestPythonRequestsPost(t *testing.T) {
	calls := parseAndExtractNetwork(t, "python", `
import requests

response = requests.post("http://api.example.com/users", json=data)
`)
	if len(calls) == 0 {
		t.Fatal("expected network call for requests.post()")
	}
	if calls[0].Method != "POST" {
		t.Errorf("expected POST, got %s", calls[0].Method)
	}
}

func TestPythonHttpx(t *testing.T) {
	calls := parseAndExtractNetwork(t, "python", `
import httpx

response = httpx.get("http://api.example.com/data")
`)
	if len(calls) == 0 {
		t.Fatal("expected network call for httpx.get()")
	}
	if calls[0].Method != "GET" {
		t.Errorf("expected GET, got %s", calls[0].Method)
	}
}

// ---------------------------------------------------------------------------
// Go HTTP client tests
// ---------------------------------------------------------------------------

func TestGoHTTPGet(t *testing.T) {
	calls := parseAndExtractNetwork(t, "go", `
package main

import "net/http"

func main() {
	resp, err := http.Get("http://example.com/api")
	_ = resp
	_ = err
}
`)
	if len(calls) == 0 {
		t.Fatal("expected network call for http.Get()")
	}
	if calls[0].Method != "GET" {
		t.Errorf("expected GET, got %s", calls[0].Method)
	}
}

func TestGoHTTPPost(t *testing.T) {
	calls := parseAndExtractNetwork(t, "go", `
package main

import "net/http"

func main() {
	resp, err := http.Post("http://example.com/api", "application/json", nil)
	_ = resp
	_ = err
}
`)
	if len(calls) == 0 {
		t.Fatal("expected network call for http.Post()")
	}
	if calls[0].Method != "POST" {
		t.Errorf("expected POST, got %s", calls[0].Method)
	}
}

// ---------------------------------------------------------------------------
// Rust HTTP client tests
// ---------------------------------------------------------------------------

func TestRustReqwest(t *testing.T) {
	calls := parseAndExtractNetwork(t, "rust", `
async fn load_data() {
    let resp = reqwest::get("http://api.example.com/data").await.unwrap();
}
`)
	if len(calls) == 0 {
		t.Errorf("expected network call for reqwest::get(); have none")
	} else if calls[0].Method != "GET" {
		t.Errorf("expected GET, got %s", calls[0].Method)
	}
}

// ---------------------------------------------------------------------------
// Java HTTP client tests
// ---------------------------------------------------------------------------

func TestJavaRestTemplate(t *testing.T) {
	calls := parseAndExtractNetwork(t, "java", `
class ApiClient {
  void fetchUsers() {
    String result = restTemplate.getForObject("http://api.example.com/users", String.class);
  }
}
`)
	if len(calls) == 0 {
		t.Errorf("expected network call for restTemplate.getForObject(); have none")
	} else if calls[0].Method != "GET" {
		t.Errorf("expected GET, got %s", calls[0].Method)
	}
}

// ---------------------------------------------------------------------------
// C# HTTP client tests
// ---------------------------------------------------------------------------

func TestCSharpHttpClient(t *testing.T) {
	calls := parseAndExtractNetwork(t, "csharp", `
class ApiClient {
  async void LoadData() {
    var response = await client.GetAsync("/api/data");
  }
}
`)
	if len(calls) == 0 {
		t.Errorf("expected network call for client.GetAsync(); have none")
	} else if calls[0].Method != "GET" {
		t.Errorf("expected GET, got %s", calls[0].Method)
	}
}

// ---------------------------------------------------------------------------
// Ruby HTTP client tests
// ---------------------------------------------------------------------------

func TestRubyHTTParty(t *testing.T) {
	calls := parseAndExtractNetwork(t, "ruby", `
response = HTTParty.get("http://api.example.com/users")
`)
	if len(calls) == 0 {
		t.Errorf("expected network call for HTTParty.get(); have none")
	} else if calls[0].Method != "GET" {
		t.Errorf("expected GET, got %s", calls[0].Method)
	}
}

// ---------------------------------------------------------------------------
// PHP HTTP client tests
// ---------------------------------------------------------------------------

func TestPHPCurl(t *testing.T) {
	calls := parseAndExtractNetwork(t, "php", `<?php
$ch = curl_init("http://api.example.com/data");
curl_exec($ch);
?>`)
	if len(calls) == 0 {
		t.Errorf("expected network calls for curl_init/curl_exec; have none")
	}
}

// ---------------------------------------------------------------------------
// Tier 2/3 tests
// ---------------------------------------------------------------------------

func TestTier2EmptyNetwork(t *testing.T) {
	for _, lang := range []string{"bash", "sql"} {
		grammar := parser.GetGrammar(lang)
		if grammar == nil {
			continue
		}
		pool := parser.NewPool(1)
		content := []byte("echo hello")
		tree, err := pool.Parse(context.Background(), content, grammar)
		pool.Shutdown()
		if err != nil {
			continue
		}
		calls := ExtractNetworkCalls(tree.RootNode(), content, nil, lang)
		if calls != nil {
			t.Errorf("expected nil for Tier 2 language %s, got %d", lang, len(calls))
		}
	}
}

func TestTier3EmptyNetwork(t *testing.T) {
	calls := ExtractNetworkCalls(nil, nil, nil, "json")
	if calls != nil {
		t.Error("expected nil for Tier 3")
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestNetworkEmptyFile(t *testing.T) {
	calls := parseAndExtractNetwork(t, "typescript", "")
	if calls != nil {
		t.Errorf("expected nil, got %d calls", len(calls))
	}
}

func TestNetworkNilRoot(t *testing.T) {
	calls := ExtractNetworkCalls(nil, nil, nil, "typescript")
	if calls != nil {
		t.Error("expected nil for nil root")
	}
}

func TestNetworkNoNetworkCalls(t *testing.T) {
	calls := parseAndExtractNetwork(t, "typescript", `
function add(a: number, b: number): number {
  return a + b;
}
`)
	if calls != nil {
		t.Errorf("expected nil for file without network calls, got %d", len(calls))
	}
}

func TestNetworkIDDeterministic(t *testing.T) {
	source := `fetch("/api/data");`
	c1 := parseAndExtractNetwork(t, "typescript", source)
	c2 := parseAndExtractNetwork(t, "typescript", source)
	if len(c1) == 0 || len(c2) == 0 {
		t.Fatal("expected network calls")
	}
	if c1[0].NetworkCallID != c2[0].NetworkCallID {
		t.Errorf("expected deterministic IDs: %s != %s", c1[0].NetworkCallID, c2[0].NetworkCallID)
	}
}
