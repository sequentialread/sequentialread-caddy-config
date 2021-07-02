package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"regexp"
	"strings"

	envOverride "git.sequentialread.com/forest/influx-style-env-override"
	isbot "zgo.at/isbot"
)

type Config struct {
	Debug                           bool
	DomainAliases                   []*DomainAlias
	IncludeDomainInKey              bool
	IncludeMethodInKey              bool
	IncludeSuccessOrFailureInKey    bool
	URIQuery                        string
	GlobalContentTypeBlacklistRegex string
	BlacklistIPsCSV                 string
	BlacklistURIRegexesCSV          string
	BlacklistHeaderKeysCSV          string
	AlwaysIncludeURIsCSV            string
	BlacklistIPs                    []string         `json:"-"`
	BlacklistURIRegexes             []*regexp.Regexp `json:"-"`
	BlacklistHeaderKeys             []string         `json:"-"`
	AlwaysIncludeURIs               []string         `json:"-"`
	GlobalContentTypeBlacklist      *regexp.Regexp   `json:"-"`
	Domains                         []*Domain
}

type Domain struct {
	MatchHostnameRegex        string
	ContentTypeWhitelistRegex string
	URIQuery                  string
	MatchHostname             *regexp.Regexp `json:"-"`
	ContentTypeWhitelist      *regexp.Regexp `json:"-"`
}

type DomainAlias struct {
	Search  string
	Replace string
}

type CaddyLog struct {
	CommonLog       string              `json:"common_log"`
	StatusCode      int                 `json:"status"`
	Request         CaddyLogRequest     `json:"request"`
	ResponseHeaders map[string][]string `json:"resp_headers"`
}

type CaddyLogRequest struct {
	RemoteAddr string              `json:"remote_addr"`
	URI        string              `json:"uri"`
	Host       string              `json:"host"`
	Proto      string              `json:"proto"`
	Method     string              `json:"method"`
	Headers    map[string][]string `json:"headers"`
}

var seenRangeRequests map[string]bool

func main() {

	seenRangeRequests := map[string]bool{}
	config := loadConfigFromFileAndEnvVars()

	bytez, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(os.Stderr, "goatcounter-caddy-log-adapter using config: %s", string(bytez))

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {

		lineText := scanner.Text()
		var caddyLog CaddyLog
		err := json.Unmarshal([]byte(lineText), &caddyLog)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to unmarshal log line because %s (line: '%s')", err, lineText)
			continue
		}

		referer := "null"
		userAgent := "null"
		contentType := ""
		if caddyLog.Request.Headers != nil {
			refererHeader, hasRefererHeader := caddyLog.Request.Headers["Referer"]
			if hasRefererHeader && len(refererHeader) > 0 {
				referer = refererHeader[0]
			}
			userAgentHeader, hasUserAgent := caddyLog.Request.Headers["User-Agent"]
			if hasUserAgent && len(userAgentHeader) > 0 {
				userAgent = userAgentHeader[0]
			}
		}
		if caddyLog.ResponseHeaders != nil {
			contentTypeHeader, hasContentTypeHeader := caddyLog.ResponseHeaders["Content-Type"]
			if hasContentTypeHeader && len(contentTypeHeader) > 0 {
				split := strings.Split(contentTypeHeader[0], ";")
				contentType = strings.TrimSpace(split[0])
			}
		}

		canonicalURI := strings.Trim(strings.ToLower(caddyLog.Request.URI), "/?")
		blacklistedURI := (func(canonicalURI string) bool {
			for _, blacklistedURIRegex := range config.BlacklistURIRegexes {
				if blacklistedURIRegex.MatchString(canonicalURI) {
					if config.Debug {
						fmt.Fprintf(os.Stderr, "%s: BlacklistURIsIgnored %s %s\n", caddyLog.Request.RemoteAddr, caddyLog.Request.Host, canonicalURI)
					}
					return true
				}
			}
			return false
		})(canonicalURI)
		if blacklistedURI {
			continue
		}

		alwaysInclude := (func(canonicalURI string) bool {
			for _, alwaysIncludeURI := range config.AlwaysIncludeURIs {
				if canonicalURI == alwaysIncludeURI {
					if config.Debug {
						fmt.Fprintf(os.Stderr, "%s: alwaysInclude %s %s\n", caddyLog.Request.RemoteAddr, caddyLog.Request.Host, canonicalURI)
					}
					return true
				}
			}
			return false
		})(canonicalURI)

		if !alwaysInclude && config.GlobalContentTypeBlacklist != nil && config.GlobalContentTypeBlacklist.MatchString(contentType) {
			if config.Debug {
				fmt.Fprintf(os.Stderr, "%s: ignored contentType: %s; matched blacklist %s  ---  %s %s\n", caddyLog.Request.RemoteAddr, contentType, config.GlobalContentTypeBlacklistRegex, caddyLog.Request.Host, caddyLog.Request.URI)
			}
			continue
		}

		requestDomain := (func() *Domain {
			for _, domain := range config.Domains {
				if domain.MatchHostname.MatchString(caddyLog.Request.Host) {
					return domain
				}
			}
			return nil
		})()

		uriQuery := config.URIQuery
		contentTypeWhitelistForDebugLog := "<no domain matched>"
		if requestDomain != nil {
			contentTypeWhitelistForDebugLog = requestDomain.ContentTypeWhitelistRegex
			if requestDomain.URIQuery != "" {
				uriQuery = requestDomain.URIQuery
			}
			if !alwaysInclude && !requestDomain.ContentTypeWhitelist.MatchString(contentType) {
				if config.Debug {
					fmt.Fprintf(os.Stderr, "%s: ignored contentType: %s; not match %s  -----  %s %s\n", caddyLog.Request.RemoteAddr, contentType, requestDomain.ContentTypeWhitelist, caddyLog.Request.Host, caddyLog.Request.URI)
				}
				continue
			}
		}

		blacklistedIP := (func(remoteAddr string) bool {
			for _, blacklistedIP := range config.BlacklistIPs {
				if strings.HasPrefix(remoteAddr, blacklistedIP) {
					if config.Debug {
						fmt.Fprintf(os.Stderr, "%s: BlacklistIPsIgnored %s %s\n", caddyLog.Request.RemoteAddr, caddyLog.Request.Host, canonicalURI)
					}
					return true
				}
			}
			return false
		})(caddyLog.Request.RemoteAddr)
		if blacklistedIP {
			continue
		}

		key := caddyLog.Request.URI

		isPrefetch := isbot.Prefetch(caddyLog.Request.Headers)
		isBotResult := isbot.UserAgent(userAgent)
		isBotReason := getIsBotReason(isBotResult)
		if !alwaysInclude && (isPrefetch || (isbot.Is(isBotResult))) {
			if config.Debug {
				fmt.Fprintf(os.Stderr, "%s: ignored cuz bot: userAgent: %s  isPrefetch: %t, isBotReason: %s\n", caddyLog.Request.RemoteAddr, userAgent, isPrefetch, isBotReason)
			}
			continue
		}

		if uriQuery == "drop" {
			split := strings.Split(key, "?")
			key = split[0]
		}

		if config.IncludeMethodInKey {
			key = fmt.Sprintf("/%s%s", caddyLog.Request.Method, key)
		}

		if config.IncludeSuccessOrFailureInKey {
			isSuccess := caddyLog.StatusCode < 400
			status := "success"
			if !isSuccess {
				status = "error"
			}
			key = fmt.Sprintf("/%s%s", status, key)
		}

		if config.IncludeDomainInKey {
			host := caddyLog.Request.Host
			if config.DomainAliases != nil {
				for _, alias := range config.DomainAliases {
					host = strings.Replace(host, alias.Search, alias.Replace, 1)
				}
			}
			key = fmt.Sprintf("/%s%s", host, key)
		}

		// don't treat each individual http range request as a separate hit. group them by goatcounter key and remote address
		// aka connection Id
		if caddyLog.Request.Headers["Range"] != nil && len(caddyLog.Request.Headers["Range"]) > 0 {
			rangeRequestKey := fmt.Sprintf("%s_%s", caddyLog.Request.RemoteAddr, key)
			if !seenRangeRequests[rangeRequestKey] {
				seenRangeRequests[rangeRequestKey] = true
			} else {
				continue
			}
		}

		search := fmt.Sprintf("\"%s %s %s\"", caddyLog.Request.Method, caddyLog.Request.URI, caddyLog.Request.Proto)
		replace := fmt.Sprintf("\"%s %s %s\"", caddyLog.Request.Method, key, caddyLog.Request.Proto)

		myCommonLog := strings.Replace(caddyLog.CommonLog, search, replace, 1)

		toPrint := fmt.Sprintf("%s:%s \"%s\" \"%s\"\n", caddyLog.Request.Host, myCommonLog, referer, userAgent)

		fmt.Fprintf(os.Stdout, toPrint)
		if config.Debug {
			fmt.Fprintf(os.Stderr, "%s: %s matched %s: isBotReason: %s %s", caddyLog.Request.RemoteAddr, contentType, contentTypeWhitelistForDebugLog, isBotReason, toPrint)
		} else {
			fmt.Fprintf(os.Stderr, "%s matched %s: isBotReason: %s %s", contentType, contentTypeWhitelistForDebugLog, isBotReason, toPrint)
		}
	}

	if err := scanner.Err(); err != nil {
		panic(err)
	}
}

func loadConfigFromFileAndEnvVars() *Config {
	configBytes, err := ioutil.ReadFile("config.json")
	if err != nil {
		panic(err)
	}
	config := &Config{
		DomainAliases: []*DomainAlias{},
		Domains:       []*Domain{},
	}
	err = json.Unmarshal(configBytes, config)
	if err != nil {
		panic(err)
	}

	// if I remember correctly I never got the silly ApplyInfluxStyleEnvOverrides thing to be able to add new entries in a list
	// so I will prepopulate the lists with 10 entries and filter the empty ones out later
	for i := 0; i < 10; i++ {
		config.DomainAliases = append(config.DomainAliases, &DomainAlias{})
	}
	for i := 0; i < 10; i++ {
		config.Domains = append(config.Domains, &Domain{})
	}

	envOverride.ApplyInfluxStyleEnvOverrides("LOGADAPTER", reflect.ValueOf(config))

	actualDomainAliases := []*DomainAlias{}
	actualDomains := []*Domain{}
	for _, alias := range config.DomainAliases {
		if alias.Search != "" {
			actualDomainAliases = append(actualDomainAliases, alias)
		}
	}
	for _, domain := range config.Domains {
		if domain.MatchHostnameRegex != "" {
			if domain.ContentTypeWhitelistRegex != "" {
				domain.ContentTypeWhitelist = regexp.MustCompile(domain.ContentTypeWhitelistRegex)
			} else {
				domain.ContentTypeWhitelist = regexp.MustCompile(".*")
			}
			domain.MatchHostname = regexp.MustCompile(domain.MatchHostnameRegex)
			actualDomains = append(actualDomains, domain)
		}
	}
	config.DomainAliases = actualDomainAliases
	config.Domains = actualDomains
	if config.GlobalContentTypeBlacklistRegex != "" {
		config.GlobalContentTypeBlacklist = regexp.MustCompile(config.GlobalContentTypeBlacklistRegex)
	}

	regexStrings := parseConfigCSV(config.BlacklistURIRegexesCSV)
	config.BlacklistURIRegexes = []*regexp.Regexp{}
	for _, regexString := range regexStrings {
		config.BlacklistURIRegexes = append(config.BlacklistURIRegexes, regexp.MustCompile(regexString))
	}
	config.AlwaysIncludeURIs = parseConfigCSV(config.AlwaysIncludeURIsCSV)
	config.BlacklistHeaderKeys = parseConfigCSV(config.BlacklistHeaderKeysCSV)
	config.BlacklistIPs = parseConfigCSV(config.BlacklistIPsCSV)

	return config
}

func getIsBotReason(code uint8) string {
	return map[uint8]string{
		0:   "Known to not be a bot",
		1:   "None of the rules matches, so probably not a bot",
		2:   "Prefetch algorithm",
		3:   "User-Agent appeared to contain a URL",
		4:   "Known client library",
		5:   "Known bot",
		6:   "User-Agent string looks \"bot-ish\"",
		7:   "User-Agent string is short",
		150: "PhantomJS headless browser",
		151: "Nightmare headless browser",
		152: "Selenium headless browser",
		153: "Generic WebDriver-based headless browser",
	}[code]
}

func parseConfigCSV(in string) []string {
	toReturn := []string{}
	raw := strings.Split(in, ",")
	for _, r := range raw {
		trimmed := strings.TrimSpace(r)
		if trimmed != "" {
			toReturn = append(toReturn, trimmed)
		}
	}
	return toReturn
}
