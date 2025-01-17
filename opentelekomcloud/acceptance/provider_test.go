package acceptance

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/opentelekomcloud/gophertelekomcloud/acceptance/tools"
	th "github.com/opentelekomcloud/gophertelekomcloud/testhelper"
	"github.com/opentelekomcloud/terraform-provider-opentelekomcloud/opentelekomcloud/helper/pathorcontents"

	"github.com/opentelekomcloud/terraform-provider-opentelekomcloud/opentelekomcloud"
	"github.com/opentelekomcloud/terraform-provider-opentelekomcloud/opentelekomcloud/common/cfg"
)

func TestProvider(t *testing.T) {
	if err := opentelekomcloud.Provider().InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestProvider_impl(t *testing.T) {
	var _ = opentelekomcloud.Provider()
}

// Steps for configuring OpenTelekomCloud with SSL validation are here:
// https://github.com/hashicorp/terraform/pull/6279#issuecomment-219020144
func TestAccProvider_caCertFile(t *testing.T) {
	if os.Getenv("TF_ACC") == "" || os.Getenv("OS_SSL_TESTS") == "" {
		t.Skip("TF_ACC or OS_SSL_TESTS not set, skipping OpenTelekomCloud SSL test.")
	}
	if os.Getenv("OS_CACERT") == "" {
		t.Skip("OS_CACERT is not set; skipping OpenTelekomCloud CA test.")
	}

	p := opentelekomcloud.Provider()

	caFile, err := envVarFile("OS_CACERT")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(caFile)

	raw := map[string]interface{}{
		"cacert_file": caFile,
	}

	if p.Configure(nil, terraform.NewResourceConfigRaw(raw)).HasError() {
		t.Fatalf("Unexpected err when specifying OpenTelekomCloud CA by file: %s", err)
	}
}

func TestAccProvider_caCertString(t *testing.T) {
	if os.Getenv("TF_ACC") == "" || os.Getenv("OS_SSL_TESTS") == "" {
		t.Skip("TF_ACC or OS_SSL_TESTS not set, skipping OpenTelekomCloud SSL test.")
	}
	if os.Getenv("OS_CACERT") == "" {
		t.Skip("OS_CACERT is not set; skipping OpenTelekomCloud CA test.")
	}

	p := opentelekomcloud.Provider()

	caContents, err := envVarContents("OS_CACERT")
	if err != nil {
		t.Fatal(err)
	}
	raw := map[string]interface{}{
		"cacert_file": caContents,
	}

	if p.Configure(nil, terraform.NewResourceConfigRaw(raw)).HasError() {
		t.Fatalf("Unexpected err when specifying OpenTelekomCloud CA by string: %s", err)
	}
}

func TestAccProvider_clientCertFile(t *testing.T) {
	if os.Getenv("TF_ACC") == "" || os.Getenv("OS_SSL_TESTS") == "" {
		t.Skip("TF_ACC or OS_SSL_TESTS not set, skipping OpenTelekomCloud SSL test.")
	}
	if os.Getenv("OS_CERT") == "" || os.Getenv("OS_KEY") == "" {
		t.Skip("OS_CERT or OS_KEY is not set; skipping OpenTelekomCloud client SSL auth test.")
	}

	p := opentelekomcloud.Provider()

	certFile, err := envVarFile("OS_CERT")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(certFile) }()
	keyFile, err := envVarFile("OS_KEY")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(keyFile) }()

	raw := map[string]interface{}{
		"cert": certFile,
		"key":  keyFile,
	}

	if p.Configure(context.Background(), terraform.NewResourceConfigRaw(raw)).HasError() {
		t.Fatalf("Unexpected err when specifying OpenTelekomCloud Client keypair by file: %s", err)
	}
}

func TestAccProvider_clientCertString(t *testing.T) {
	if os.Getenv("TF_ACC") == "" || os.Getenv("OS_SSL_TESTS") == "" {
		t.Skip("TF_ACC or OS_SSL_TESTS not set, skipping OpenTelekomCloud SSL test.")
	}
	if os.Getenv("OS_CERT") == "" || os.Getenv("OS_KEY") == "" {
		t.Skip("OS_CERT or OS_KEY is not set; skipping OpenTelekomCloud client SSL auth test.")
	}

	p := opentelekomcloud.Provider()

	certContents, err := envVarContents("OS_CERT")
	if err != nil {
		t.Fatal(err)
	}
	keyContents, err := envVarContents("OS_KEY")
	if err != nil {
		t.Fatal(err)
	}

	raw := map[string]interface{}{
		"cert": certContents,
		"key":  keyContents,
	}

	if p.Configure(context.Background(), terraform.NewResourceConfigRaw(raw)).HasError() {
		t.Fatalf("Unexpected err when specifying OpenTelekomCloud Client keypair by contents: %s", err)
	}
}

func envVarContents(varName string) (string, error) {
	contents, _, err := pathorcontents.Read(os.Getenv(varName))
	if err != nil {
		return "", fmt.Errorf("error reading %s: %s", varName, err)
	}
	return contents, nil
}

func envVarFile(varName string) (string, error) {
	contents, err := envVarContents(varName)
	if err != nil {
		return "", err
	}

	tmpFile, err := ioutil.TempFile("", varName)
	if err != nil {
		return "", fmt.Errorf("error creating temp file: %s", err)
	}
	if _, err := tmpFile.Write([]byte(contents)); err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", fmt.Errorf("error writing temp file: %s", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", fmt.Errorf("error closing temp file: %s", err)
	}
	return tmpFile.Name(), nil
}

func TestLoadAndValidate_cloud(t *testing.T) {
	cloudName := "terraform-test"
	cloudsYamlFile := filepath.Join("/tmp",
		fmt.Sprintf("%s.yaml", acctest.RandString(5)))
	secureYamlFile := filepath.Join("/tmp",
		fmt.Sprintf("%s.yaml", acctest.RandString(5)))
	password := acctest.RandString(16)
	projectName := acctest.RandString(10)
	cloudsConfig := fmt.Sprintf(`
clouds:
  %s:
    auth:
      auth_url: https://iam.eu-de.otc.t-systems.com/v3
      username: myuser
      project_name: "%s"
      domain_name: OTCMY1000000000066
`, cloudName, projectName)
	secureConfig := fmt.Sprintf(`
clouds:
  %s:
    auth:
      password: %s
`, cloudName, password)

	th.AssertNoErr(t, ioutil.WriteFile(cloudsYamlFile, []byte(cloudsConfig), 0755))
	defer func() {
		th.AssertNoErr(t, os.Remove(cloudsYamlFile))
	}()
	th.AssertNoErr(t, ioutil.WriteFile(secureYamlFile, []byte(secureConfig), 0755))
	defer func() {
		th.AssertNoErr(t, os.Remove(secureYamlFile))
	}()

	_ = os.Setenv("OS_CLIENT_CONFIG_FILE", cloudsYamlFile)
	_ = os.Setenv("OS_CLIENT_SECURE_FILE", secureYamlFile)

	config := cfg.Config{
		Cloud: cloudName,
	}
	err := config.Load()
	th.AssertNoErr(t, err)

	th.AssertEquals(t, projectName, config.TenantName)
	th.AssertEquals(t, password, config.Password)
}

func TestLoadAndValidate_errors(t *testing.T) {
	type negativeConfig struct {
		cfg.Config
		ErrorRegex string
	}

	cases := map[string]negativeConfig{
		"No Identity Endpoint": {
			ErrorRegex: `one of 'auth_url' or 'cloud' must be`,
		},
		"No Project ID/Name": {
			Config: cfg.Config{
				IdentityEndpoint: "asd",
			},
			ErrorRegex: `no project name/id.+is provided`,
		},
		"No Credentials": {
			Config: cfg.Config{
				IdentityEndpoint: "asd",
				TenantID:         tools.RandomString("id-", 10),
				TenantName:       tools.RandomString("name-", 10),
			},
			ErrorRegex: "no auth means provided",
		},
		"Invalid Endpoint": {
			Config: cfg.Config{
				IdentityEndpoint: "asd",
				EndpointType:     "invalid",
			},
			ErrorRegex: "invalid endpoint type provided",
		},
	}

	for name, config := range cases {
		t.Run(name, func(st *testing.T) {
			regex, rErr := regexp.Compile(config.ErrorRegex)
			if rErr != nil {
				st.Fatalf("invalid error regexp: %s", config.ErrorRegex)
			}
			err := config.LoadAndValidate()
			if err == nil {
				st.Fatalf("error was expected to happen")
			}
			if !regex.MatchString(err.Error()) {
				st.Fatalf("error `%s` doesn't match regex: `%s`", err, regex)
			}
		})
	}
}
