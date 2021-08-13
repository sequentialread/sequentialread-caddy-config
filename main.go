package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	errors "git.sequentialread.com/forest/pkg-errors"
)

type DockerContainer struct {
	Id              string
	Names           []string
	Labels          map[string]string
	NetworkSettings DockerContainerNetworkSettings
}

func (container DockerContainer) GetDisplayName() string {
	if container.Names != nil && len(container.Names) > 0 {
		return fmt.Sprintf("%s (%s)", container.Names[0], container.Id)
	} else {
		return container.Id
	}
}

func (container DockerContainer) GetShortName() string {
	if container.Names != nil && len(container.Names) > 0 {
		return container.Names[0]
	} else {
		return container.Id
	}
}

type DockerContainerNetworkSettings struct {
	Networks map[string]DockerContainerNetwork
}

type DockerContainerNetwork struct {
	NetworkID string
	IPAddress string
}

type ContainerConfig struct {
	PublicPort           int
	PublicProtocol       string
	PublicHostnames      string
	PublicPaths          string
	ContainerProtocol    string
	ContainerAddress     string
	ContainerName        string
	HaProxyProxyProtocol bool
}

type CaddyApp struct {
	//http app
	Servers map[string]*CaddyServer `json:"servers,omitempty"`

	//tls app
	Automation *CaddyTLSAutomation `json:"automation,omitempty"`
}

type CaddyTLSAutomation struct {
	Policies []CaddyTLSPolicy `json:"policies,omitempty"`
}

type CaddyTLSPolicy struct {
	Subjects []string          `json:"subjects,omitempty"`
	Issuers  []CaddyACMEIssuer `json:"issuers,omitempty"`
}

type CaddyACMEIssuer struct {
	CA     string `json:"ca"`
	Email  string `json:"email"`
	Module string `json:"module"`
}

type CaddyServer struct {
	Listen []string         `json:"listen"`
	Routes []CaddyRoute     `json:"routes"`
	Logs   *CaddyServerLogs `json:"logs"`
}

type CaddyServerLogs struct {
	LoggerNames map[string]string `json:"logger_names"`
}

type CaddyRoute struct {
	Handle   []CaddyHandler `json:"handle,omitempty"`
	Match    []CaddyMatch   `json:"match,omitempty"`
	Terminal bool           `json:"terminal,omitempty"`
}

// https://caddyserver.com/docs/json/apps/http/servers/routes/handle/
type CaddyHandler struct {
	Handler string `json:"handler"`

	//CaddySubrouteHandler
	Routes []CaddyRoute `json:"routes,omitempty"`

	//CaddyReverseProxyHandler
	Upstreams []CaddyUpstream `json:"upstreams,omitempty"`

	//CaddyStaticResponseHandler
	Headers    map[string][]string `json:"headers,omitempty"`
	StatusCode int                 `json:"status_code,omitempty"`

	//CaddyFileServerHandler
	Root        string `json:"root,omitempty"`
	Passthrough bool   `json:"pass_thru,omitempty"`
}

// https://caddyserver.com/docs/json/apps/http/servers/routes/handle/reverse_proxy/
type CaddyUpstream struct {
	Dial string `json:"dial"`
}

// https://caddyserver.com/docs/json/apps/http/servers/routes/match/
type CaddyMatch struct {
	// match host
	Host []string `json:"host,omitempty"`

	// match path
	Path []string `json:"path,omitempty"`

	// match vars regexp
	VarsRegexp map[string]CaddyVarsRegexp `json:"vars_regexp,omitempty"`
}

type CaddyVarsRegexp struct {
	Name    string `json:"name,omitempty"`
	Pattern string `json:"pattern,omitempty"`
}

var CADDY_SOCKET = "/caddysocket/caddy.sock"
var DOCKER_SOCKET = "/var/run/docker.sock"
var FAVICON_DIRECTORY = "/srv/static"
var DOCKER_API_VERSION = "v1.40"
var CADDY_ACME_ISSUER_URL = "https://acme-v02.api.letsencrypt.org/directory"
var CADDY_ACME_CLIENT_EMAIL_ADDRESS = ""

const POLLING_INTERVAL = time.Second * time.Duration(5)

var dockerHTTPClient *http.Client
var caddyHTTPClient *http.Client

func main() {

	CADDY_SOCKET = getEnvVar("$CADDY_SOCKET", CADDY_SOCKET)
	DOCKER_SOCKET = getEnvVar("$DOCKER_SOCKET", DOCKER_SOCKET)
	FAVICON_DIRECTORY = getEnvVar("$FAVICON_DIRECTORY", FAVICON_DIRECTORY)
	DOCKER_API_VERSION = getEnvVar("$DOCKER_API_VERSION", DOCKER_API_VERSION)
	CADDY_ACME_ISSUER_URL = getEnvVar("$CADDY_ACME_ISSUER_URL", CADDY_ACME_ISSUER_URL)
	CADDY_ACME_CLIENT_EMAIL_ADDRESS = getEnvVar("$CADDY_ACME_CLIENT_EMAIL_ADDRESS", CADDY_ACME_CLIENT_EMAIL_ADDRESS)

	if CADDY_ACME_ISSUER_URL == "" || CADDY_ACME_CLIENT_EMAIL_ADDRESS == "" {
		log.Printf("using default caddy zerossl configuration. Set the caddy acme environment variables to override this.")
	}

	dockerHTTPClient = &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", DOCKER_SOCKET)
			},
		},
		Timeout: time.Second * time.Duration(5),
	}

	caddyHTTPClient = &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", CADDY_SOCKET)
			},
		},
		Timeout: time.Second * time.Duration(5),
	}

	for {
		err := IngressConfig()
		if err != nil {
			log.Printf("could not update caddy config: %v\n", err)
		}
		time.Sleep(POLLING_INTERVAL)
	}
}

var previousCaddyConfigBytes []byte

func IngressConfig() error {

	containers, err := ListDockerContainers()
	if err != nil {
		return errors.Wrap(err, "can't list docker containers")
	}

	// sequentialread-80-public-port: 443
	// sequentialread-80-public-protocol: https
	// sequentialread-80-public-hostnames: "example.com,www.example.com"
	// sequentialread-80-public-paths: "/example,/example2"
	// sequentialread-80-container-protocol: http
	ingressLabelRegexp := regexp.MustCompile("sequentialread-([0-9]+)-((public-port)|(public-protocol)|(public-hostnames)|(public-paths)|(container-protocol))")

	containerConfigs := map[string]*ContainerConfig{}

	for _, container := range containers {
		ipAddress := ""
		for _, containerNetwork := range container.NetworkSettings.Networks {
			// TODO filter this to only networks that the caddy container has access to?
			ipAddress = containerNetwork.IPAddress
			break
		}
		for key, value := range container.Labels {
			matches := ingressLabelRegexp.FindAllStringSubmatch(key, -1)
			if strings.HasPrefix(key, "sequentialread") && len(matches) == 0 {
				return errors.Wrapf(
					err, "failed to parse container %s ingress label '%s'. please refer to the documentation for valid label formats (TODO include a link here)",
					container.GetDisplayName(), key,
				)
			}
			if len(matches) > 0 {
				port, _ := strconv.Atoi(matches[0][1])
				labelType := matches[0][2]

				// TODO if the container is stopped, just skip it..
				if ipAddress == "" {
					return fmt.Errorf(
						"container %s has an ingress label '%s' but it doesn't have an IP address on the ingress network",
						container.GetDisplayName(), key,
					)
				}

				if _, has := containerConfigs[container.Id]; !has {
					containerConfigs[container.Id] = &ContainerConfig{
						ContainerAddress: fmt.Sprintf("%s:%d", ipAddress, port),
						ContainerName:    container.GetShortName(),
					}
				}

				if labelType == "public-protocol" {
					containerConfigs[container.Id].PublicProtocol = value
				}
				if labelType == "public-port" {
					port, err := strconv.Atoi(value)
					if err != nil {
						return errors.Wrapf(
							err, "container %s public-port ingress label must be an integer ('%s' was given)",
							container.GetDisplayName(), value,
						)
					}
					containerConfigs[container.Id].PublicPort = port
				}
				if labelType == "public-hostnames" {
					containerConfigs[container.Id].PublicHostnames = value
				}
				if labelType == "public-paths" {
					containerConfigs[container.Id].PublicPaths = value
				}
				if labelType == "container-protocol" {
					containerConfigs[container.Id].ContainerProtocol = value
				}
			}
		}
	}

	caddyConfig := map[string]*CaddyApp{}
	publicPorts := map[int][]*ContainerConfig{}

	for _, x := range containerConfigs {
		if x.PublicPort == 0 {
			return fmt.Errorf(
				"found ingress label for container %s but it is missing the required public-port label",
				x.ContainerName,
			)
		}

		if _, has := publicPorts[x.PublicPort]; !has {
			publicPorts[x.PublicPort] = []*ContainerConfig{}
		}
		publicPorts[x.PublicPort] = append(publicPorts[x.PublicPort], x)
	}

	// TODO sort public ports once we get that far and have more than one

	for port, containerConfigs := range publicPorts {
		if port == 443 {

			allHostnames := []string{}
			for _, container := range containerConfigs {
				allHostnames = append(allHostnames, strings.Split(container.PublicHostnames, ",")...)
			}
			sort.Strings(allHostnames)

			// facebook adds this ?fbclid=xyz request parameter whenever someone clicks a link
			// this handler will match all requests that have this parameter
			// and it will redirect to the same URI with the parameter removed
			fbclidRoute := CaddyRoute{
				Handle: []CaddyHandler{
					CaddyHandler{
						Handler: "static_response",
						Headers: map[string][]string{
							"Location": []string{"{http.regexp.fbclid_regex.1}"},
						},
						StatusCode: 302,
					},
				},
				Match: []CaddyMatch{
					CaddyMatch{
						VarsRegexp: map[string]CaddyVarsRegexp{
							"{http.request.uri}": CaddyVarsRegexp{
								Name:    "fbclid_regex",
								Pattern: "^(.*?)([?&]fbclid=[a-zA-Z0-9_-]+)$",
							},
						},
					},
				},
			}

			caddyConfig["http"] = &CaddyApp{
				Servers: map[string]*CaddyServer{
					"srv0": {
						Listen: []string{":443"},
						Logs: &CaddyServerLogs{
							LoggerNames: map[string]string{
								"*": "goatcounter",
							},
						},
						Routes: []CaddyRoute{fbclidRoute},
					},
				},
			}
			if CADDY_ACME_ISSUER_URL != "" && CADDY_ACME_CLIENT_EMAIL_ADDRESS != "" {
				caddyConfig["tls"] = &CaddyApp{
					Automation: &CaddyTLSAutomation{
						Policies: []CaddyTLSPolicy{
							CaddyTLSPolicy{
								Subjects: allHostnames,
								Issuers: []CaddyACMEIssuer{
									CaddyACMEIssuer{
										CA:     CADDY_ACME_ISSUER_URL,
										Email:  CADDY_ACME_CLIENT_EMAIL_ADDRESS,
										Module: "acme",
									},
								},
							},
						},
					},
				}
			}

			// sort them so that the json always comes out the same & easier to compare (canonical)
			// also include the ones that specify a path first so they take precidence when two handlers match the same domain
			getShortestPathLength := func(paths []string) int {
				shortest := 255
				for _, path := range paths {
					result := len(strings.Split(path, "/"))
					if result < shortest {
						shortest = result
					}
				}
				return shortest
			}
			sort.Slice(containerConfigs, func(i, j int) bool {
				sortI := fmt.Sprintf(
					"%s%s",
					string(rune(255-getShortestPathLength(strings.Split(containerConfigs[i].PublicPaths, ",")))),
					containerConfigs[i].ContainerName,
				)
				sortJ := fmt.Sprintf(
					"%s%s",
					string(rune(255-getShortestPathLength(strings.Split(containerConfigs[j].PublicPaths, ",")))),
					containerConfigs[j].ContainerName,
				)
				return sortI < sortJ
			})

			for _, container := range containerConfigs {
				if container.PublicHostnames != "" {
					match := CaddyMatch{
						Host: strings.Split(container.PublicHostnames, ","),
					}
					if container.PublicPaths != "" {
						match.Path = strings.Split(container.PublicPaths, ",")
					}

					newRoute := CaddyRoute{
						Handle: []CaddyHandler{
							// this handler is just here to standardize the favicon (or any other universal static file)
							// across the sites
							{
								Handler:     "file_server",
								Root:        FAVICON_DIRECTORY,
								Passthrough: true,
							},
							{
								Handler: "reverse_proxy",
								Upstreams: []CaddyUpstream{
									{
										Dial: container.ContainerAddress,
									},
								},
							},
						},
						Match:    []CaddyMatch{match},
						Terminal: true,
					}

					caddyConfig["http"].Servers["srv0"].Routes = append(
						caddyConfig["http"].Servers["srv0"].Routes,
						newRoute,
					)
				}
			}
		} else {
			// TODO support TCP and TLS (udp??)
			return fmt.Errorf(
				"unsupported public-port %d on container '%s'. currently only https is supported",
				port, containerConfigs[0].ContainerName,
			)
		}
	}

	caddyConfigBytes, _ := json.MarshalIndent(caddyConfig, "", "  ")

	caddyResponseString := ""
	_, caddyResponseBytes, err := unixHTTP(
		caddyHTTPClient, CADDY_SOCKET, "GET", "/config/apps", caddyConfigBytes,
	)
	if err != nil {
		caddyResponseString = string(caddyResponseBytes)
	}
	caddyConfigIsEmpty := caddyResponseString == "null" || caddyResponseString == "[]"

	if caddyConfigIsEmpty || !byteArraysEqual(caddyConfigBytes, previousCaddyConfigBytes) {

		log.Println("!byteArraysEqual(caddyConfigBytes, previousCaddyConfigBytes)")
		log.Println(".")
		log.Println(".")
		log.Println(".")
		log.Printf("==================================\nOLD\n%s\n\n", string(previousCaddyConfigBytes))
		log.Printf("==================================\nNEW\n%s\n\n", string(caddyConfigBytes))
		log.Println("")
		log.Println("")
		log.Println("")

		caddyResponse, caddyResponseBytes, err := unixHTTP(
			caddyHTTPClient, CADDY_SOCKET, "POST", "/config/apps", caddyConfigBytes,
		)
		if err != nil {
			return errors.Wrap(err, "failed to call caddy admin api")
		}
		if caddyResponse.StatusCode != http.StatusOK {
			return fmt.Errorf("caddy admin api returned HTTP %d: %s", caddyResponse.StatusCode, string(caddyResponseBytes))
		}

		previousCaddyConfigBytes = caddyConfigBytes
	}

	return nil
}

func byteArraysEqual(a, b []byte) bool {
	if a == nil || b == nil {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i, b_b := range b {
		if a[i] != b_b {
			return false
		}
	}
	return true
}

//https://docs.docker.com/engine/api/v1.40/#tag/Container
func ListDockerContainers() ([]DockerContainer, error) {
	bytes, err := myDockerGet("containers/json?all=true")
	if err != nil {
		return nil, err
	}
	var containers []DockerContainer
	err = json.Unmarshal(bytes, &containers)
	if err != nil {
		return nil, errors.Wrap(err, "docker API json parse error")
	}
	return containers, nil
}

func myDockerGet(endpoint string) ([]byte, error) {

	response, bytes, err := unixHTTP(
		dockerHTTPClient, DOCKER_SOCKET, "GET", fmt.Sprintf("/%s/%s", DOCKER_API_VERSION, endpoint), nil,
	)

	if err != nil {
		return nil, errors.Wrap(err, "can't talk to docker api")
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"docker api (%s) returned HTTP %d: %s",
			endpoint, response.StatusCode, string(bytes))
	}

	return bytes, nil
}

func unixHTTP(unixHTTPClient *http.Client, socket, method, endpoint string, body []byte) (*http.Response, []byte, error) {
	request, err := http.NewRequest(method, fmt.Sprintf("http://localhost%s", endpoint), bytes.NewReader(body))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "unixHTTP %s could not create request object (%s)", endpoint, socket)
	}
	if body != nil {
		request.Header.Add("content-type", "application/json")
	}
	response, err := unixHTTPClient.Do(request)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "unixHTTP %s failed (%s)", endpoint, socket)
	}
	bytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "unixHTTP %s read error (%s)", endpoint, socket)
	}
	return response, bytes, nil
}

func getEnvVar(expand, defaultValue string) string {
	result := os.ExpandEnv(expand)
	if result != "" {
		return result
	}
	return defaultValue
}
