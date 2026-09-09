// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/rrmcguinness/modenv/pkg/modenv"
)

func main() {
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		printHelp(stdout)
		return 1
	}

	command := args[1]
	switch command {
	case "setup":
		return runSetup(stdout, stderr)
	case "read":
		return runRead(stdout, stderr)
	case "encode", "--encode":
		return runEncode(args, stdin, stdout, stderr)
	case "help", "-h", "--help":
		printHelp(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "Unknown command: %q\n", command)
		printHelp(stdout)
		return 1
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage: modenv <command> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  setup                         Create initial configuration files if they do not exist")
	fmt.Fprintln(w, "  read                          Show the resolved configuration tree for the current MODENV_RUNTIME")
	fmt.Fprintln(w, "  encode [flags]                Encode a secret value via secure interactive prompt")
	fmt.Fprintln(w, "    --type, -t <simple|pks>     Encryption type (default: simple)")
	fmt.Fprintln(w, "    --public-key, -k <path>     RSA public key PEM file for pks type")
	fmt.Fprintln(w, "    --legacy                    Produce legacy xor: prefix")
	fmt.Fprintln(w, "  --encode                      Alias for encode")
	fmt.Fprintln(w, "  help                          Show this help message")
}

func runSetup(stdout, stderr io.Writer) int {
	files := map[string]string{
		".env.toml": `app_name = "my-app"
port = 8080
features = ["api"]

# [secrets_store]
# type = "simple" # "simple" | "pks" | "cloud"
# google_cloud_project = "my-gcp-project"
# google_cloud_region = "us-central1"
# key_location = "keys/private.pem"

[database]
host = "localhost"
user = "root"
password = "plain_db_password"
`,
		".env.local.toml": `# Local environment overrides (never commit this file)
port = 3000

[database]
host = "127.0.0.1"
password = "simple://01000704022949063a1600061f03421901" # Encoded: "local_db_password"
`,
		".env.development.toml": `port = 8000

[database]
host = "dev-db"
`,
		".env.production.toml": `port = 9000

[database]
host = "prod-db-cluster"
`,
	}

	for filename, content := range files {
		destPath := resolvePath(filename)
		if fileExists(destPath) {
			fmt.Fprintf(stdout, "Skipping %s (already exists)\n", destPath)
			continue
		}

		dir := filepath.Dir(destPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(stderr, "Error creating directory %s: %v\n", dir, err)
			return 1
		}

		err := os.WriteFile(destPath, []byte(content), 0644)
		if err != nil {
			fmt.Fprintf(stderr, "Error creating %s: %v\n", destPath, err)
			return 1
		}
		fmt.Fprintf(stdout, "Created %s\n", destPath)
	}
	return 0
}

func runRead(stdout, stderr io.Writer) int {
	baseFile := resolvePath(".env.toml")
	if !fileExists(baseFile) {
		fmt.Fprintf(stderr, "Error: Base configuration file %s is missing. Run 'modenv setup' to initialize configuration files.\n", baseFile)
		return 1
	}

	cfg, err := modenv.Load(nil)
	if err != nil {
		fmt.Fprintf(stderr, "Error loading configuration: %v\n", err)
		return 1
	}

	runtime := os.Getenv("MODENV_RUNTIME")
	if runtime == "" {
		runtime = "(none)"
	}
	prefix := os.Getenv("MODENV_PREFIX")
	if prefix == "" {
		prefix = "(working directory)"
	}
	fmt.Fprintf(stdout, "# Resolved Configuration Tree (MODENV_RUNTIME=%s, MODENV_PREFIX=%s)\n", runtime, prefix)

	err = toml.NewEncoder(stdout).Encode(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "Error encoding resolved configuration: %v\n", err)
		return 1
	}
	return 0
}

func runEncode(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var encType = "simple"
	var pubKeyPath = ""
	var legacy = false
	var hasPositionalArg = false

	for i := 2; i < len(args); i++ {
		arg := args[i]
		if arg == "--legacy" {
			legacy = true
		} else if strings.HasPrefix(arg, "--type=") {
			encType = strings.TrimPrefix(arg, "--type=")
		} else if (arg == "-t" || arg == "--type") && i+1 < len(args) {
			i++
			encType = args[i]
		} else if strings.HasPrefix(arg, "--public-key=") {
			pubKeyPath = strings.TrimPrefix(arg, "--public-key=")
		} else if (arg == "-k" || arg == "--public-key") && i+1 < len(args) {
			i++
			pubKeyPath = args[i]
		} else if !strings.HasPrefix(arg, "-") {
			hasPositionalArg = true
		}
	}

	if hasPositionalArg {
		fmt.Fprintln(stderr, "Error: Passing secret values directly on the command line is disabled to prevent exposure in shell history.")
		fmt.Fprintln(stderr, "Please run 'modenv encode' without a secret argument to be prompted securely.")
		return 1
	}

	if encType != "simple" && encType != "pks" {
		fmt.Fprintf(stderr, "Error: Unknown encryption type %q (supported: simple, pks)\n", encType)
		return 1
	}

	var pubKeyPEM string
	if encType == "pks" {
		if pubKeyPath != "" {
			data, err := os.ReadFile(resolvePath(pubKeyPath))
			if err != nil {
				fmt.Fprintf(stderr, "Error reading public key file %s: %v\n", pubKeyPath, err)
				return 1
			}
			pubKeyPEM = string(data)
		} else if envKey := os.Getenv("MODENV_PUBLIC_KEY"); envKey != "" {
			pubKeyPEM = envKey
		} else {
			fmt.Fprintf(stderr, "Error: PKS encryption requires --public-key=<path> or MODENV_PUBLIC_KEY environment variable\n")
			return 1
		}
	}

	reader := bufio.NewReader(stdin)

	secret, err := readPassword("Enter secret: ", reader, stdin, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "Error reading secret: %v\n", err)
		return 1
	}
	if secret == "" {
		fmt.Fprintln(stderr, "Error: Secret cannot be empty")
		return 1
	}

	confirm, err := readPassword("Confirm secret: ", reader, stdin, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "Error reading confirmation: %v\n", err)
		return 1
	}
	if secret != confirm {
		fmt.Fprintln(stderr, "Error: Secrets do not match")
		return 1
	}

	if legacy {
		fmt.Fprintln(stdout, modenv.EncryptLegacySecret(secret))
		return 0
	}

	switch encType {
	case "simple":
		fmt.Fprintln(stdout, modenv.EncryptSecret(secret))
		return 0
	case "pks":
		encoded, err := modenv.EncryptPKSSecret(secret, pubKeyPEM)
		if err != nil {
			fmt.Fprintf(stderr, "Error encrypting secret: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, encoded)
		return 0
	default:
		return 1
	}
}

func readPassword(prompt string, reader *bufio.Reader, stdin io.Reader, promptWriter io.Writer) (string, error) {
	if promptWriter != nil {
		fmt.Fprint(promptWriter, prompt)
	}

	if f, ok := stdin.(*os.File); ok {
		cmd := exec.Command("stty", "-echo")
		cmd.Stdin = f
		if err := cmd.Run(); err == nil {
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt)
			defer func() {
				signal.Stop(sigChan)
				cmdEcho := exec.Command("stty", "echo")
				cmdEcho.Stdin = f
				_ = cmdEcho.Run()
				if promptWriter != nil {
					fmt.Fprintln(promptWriter)
				}
			}()
			go func() {
				if _, ok := <-sigChan; ok {
					cmdEcho := exec.Command("stty", "echo")
					cmdEcho.Stdin = f
					_ = cmdEcho.Run()
					os.Exit(130)
				}
			}()
		}
	}

	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func resolvePath(filename string) string {
	prefix := os.Getenv("MODENV_PREFIX")
	if prefix == "" {
		return filename
	}
	return filepath.Join(prefix, filename)
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
