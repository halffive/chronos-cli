package auth_test

import (
	"os"
	"strings"
	"testing"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/config"
)

const key = clitest.APIKey

func account() clitest.Response {
	return clitest.OK(clitest.Fixture("account"))
}

func unauthorized() clitest.Response {
	return clitest.Failure{Status: 401, Code: "unauthorized", Message: "Unauthorized"}.Response()
}

func saveInKeychain(t *testing.T, dir string, keychain *clitest.Keychain, baseURL, secret string) {
	t.Helper()
	if _, err := config.SaveAPIKey(config.SaveOptions{Dir: dir, Keychain: keychain, BaseURL: baseURL, APIKey: secret}); err != nil {
		t.Fatal(err)
	}
}

func golden(t *testing.T, name string, api *clitest.API, got string) {
	t.Helper()
	host := strings.TrimPrefix(api.URL, "http://")
	clitest.Golden(t, name, strings.ReplaceAll(got, host, "127.0.0.1:PORT"))
}

func readConfig(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(config.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func assertNoKey(t *testing.T, res clitest.Result) {
	t.Helper()
	for name, out := range map[string]string{"stdout": res.Stdout, "stderr": res.Stderr} {
		if strings.Contains(out, key) {
			t.Errorf("%s contains the full key:\n%s", name, out)
		}
	}
}
