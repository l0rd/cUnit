package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/Sirupsen/logrus"
	"github.com/l0rd/docker-unit/build"
)

const (
	defaultDockerSocket       = "unix:///var/run/docker.sock"
	defaultCertDir            = "$HOME/.docker"
	defaultCACertFilename     = "ca.pem"
	defaultClientCertFilename = "cert.pem"
	defaultClientKeyFilename  = "key.pem"
)

func main() {
	// Set Docker connection flags.
	var (
		daemonURL      = flag.String("H", "", "Docker daemon socket/host to connect to")
		useTLS         = flag.Bool("-tls", false, "Use TLS client cert/key (implied by --tlsverify)")
		verifyTLS      = flag.Bool("-tlsverify", true, "Use TLS and verify the remote server certificate")
		caCertFile     = flag.String("-cacert", "", "Trust certs signed only by this CA")
		clientCertFile = flag.String("-cert", "", "TLS client certificate")
		clientKeyFile  = flag.String("-key", "", "TLS client key")
	)

	// Build context flags.
	var (
		contextDirectory = flag.String("C", ".", "Build context directory")
		dockerfilePath   = flag.String("f", "", "Path to Dockerfile")
		repoTag          = flag.String("t", "", "Repository name (and optionally a tag) for the image")
	)

	debug := flag.Bool("d", false, "enable debug output")

	flag.Parse()

	if *debug {
		log.SetLevel(log.DebugLevel)
	}
    
    docker := dockerClientConnection {
        daemonURL: *daemonURL,
        useTLS: *useTLS,
        verifyTLS: *verifyTLS,
        caCertFile: *caCertFile,
        clientCertFile: *clientCertFile,
        clientKeyFile: *clientKeyFile,
    }
    
    getDockerClientConnection(&docker)

	/***************
	 * Begin Build *
	 ***************/

	builder, err := build.NewBuilder(docker.daemonURL, docker.tlsConfig, *contextDirectory, *dockerfilePath, *repoTag)
	if err != nil {
		log.Fatalf("unable to initialize builder: %s", err)
	}

	if err := builder.Run(); err != nil {
		log.Fatal(err)
	}
}

type dockerClientConnection struct {
		daemonURL string
		useTLS bool
		verifyTLS bool
		caCertFile string
		clientCertFile string
		clientKeyFile string
        tlsConfig *tls.Config
}

func getDockerClientConnection(docker *dockerClientConnection) {
    /********************************
	 * Get Docker client connection *
	 ********************************/

	// Command line option takes preference, then fallback to environment var,
	// then fallback to default.
	if docker.daemonURL == "" {
		if docker.daemonURL = os.Getenv("DOCKER_HOST"); docker.daemonURL == "" {
			docker.daemonURL = defaultDockerSocket
		}
	}

	// Setup TLS config.
	//var tlsConfig *tls.Config
	if docker.useTLS || docker.verifyTLS || os.Getenv("DOCKER_TLS_VERIFY") != "" {
		docker.tlsConfig = &tls.Config{
			InsecureSkipVerify: !docker.verifyTLS,
		}

		// Get the cert path specified by environment variable or default.
		certDir := os.Getenv("DOCKER_CERT_PATH")
		if certDir == "" {
			certDir = defaultCertDir
		}
		certDir = os.ExpandEnv(certDir)

		// Get CA cert bundle.
		if docker.caCertFile == "" { // Not set on command line.
			docker.caCertFile = filepath.Join(certDir, defaultCACertFilename)
			if _, err := os.Stat(docker.caCertFile); os.IsNotExist(err) {
				// CA cert bundle does not exist in default location.
				// We'll use the system default root CAs instead.
				docker.caCertFile = ""
			}
		}

		if docker.caCertFile != "" {
			certBytes, err := ioutil.ReadFile(docker.caCertFile)
			if err != nil {
				log.Fatalf("unable to read ca cert file: %s", err)
			}

			docker.tlsConfig.RootCAs = x509.NewCertPool()
			if !docker.tlsConfig.RootCAs.AppendCertsFromPEM(certBytes) {
				log.Fatal("unable to load ca cert file")
			}
		}

		// Get client cert.
		if docker.clientCertFile == "" { // Not set on command line.
			docker.clientCertFile = filepath.Join(certDir, defaultClientCertFilename)
			if _, err := os.Stat(docker.clientCertFile); os.IsNotExist(err) {
				// Client cert does not exist in default location.
				docker.clientCertFile = ""
			}
		}

		// Get client key.
		if docker.clientKeyFile == "" { // Not set on commadn line.
			docker.clientKeyFile = filepath.Join(certDir, defaultClientKeyFilename)
			if _, err := os.Stat(docker.clientKeyFile); os.IsNotExist(err) {
				// Client key does not exist in default location.
				docker.clientKeyFile = ""
			}
		}

		// If one of client cert/key is specified then both must be.
		certSpecified := docker.clientCertFile != ""
		keySpecified := docker.clientKeyFile != ""
		if certSpecified != keySpecified {
			log.Fatal("must specify both client certificate and key")
		}

		// If both are specified, load them into the tls config.
		if certSpecified && keySpecified {
			tlsClientCert, err := tls.LoadX509KeyPair(docker.clientCertFile, docker.clientKeyFile)
			if err != nil {
				log.Fatalf("unable to load client cert/key pair: %s", err)
			}

			docker.tlsConfig.Certificates = append(docker.tlsConfig.Certificates, tlsClientCert)
		}
	}
}
