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
)

type Config struct {
	Debug                           bool
	DomainAliases                   []*DomainAlias
	IncludeDomainInKey              bool
	IncludeMethodInKey              bool
	IncludeSuccessOrFailureInKey    bool
	URIQuery                        string
	GlobalContentTypeBlacklistRegex string
	GlobalContentTypeBlacklist      *regexp.Regexp `json:"-"`
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
	URI     string              `json:"uri"`
	Host    string              `json:"host"`
	Proto   string              `json:"proto"`
	Method  string              `json:"method"`
	Headers map[string][]string `json:"headers"`
}

func main() {

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

		if config.GlobalContentTypeBlacklist != nil && config.GlobalContentTypeBlacklist.MatchString(contentType) {
			if config.Debug {
				fmt.Fprintf(os.Stderr, "ignored contentType: %s; matched blacklist %s\n", contentType, config.GlobalContentTypeBlacklistRegex)
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
			if !requestDomain.ContentTypeWhitelist.MatchString(contentType) {
				if config.Debug {
					fmt.Fprintf(os.Stderr, "ignored contentType: %s; not match %s\n", contentType, requestDomain.ContentTypeWhitelist)
				}
				continue
			}
		}

		key := caddyLog.Request.URI
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

		search := fmt.Sprintf("\"%s %s %s\"", caddyLog.Request.Method, caddyLog.Request.URI, caddyLog.Request.Proto)
		replace := fmt.Sprintf("\"%s %s %s\"", caddyLog.Request.Method, key, caddyLog.Request.Proto)

		myCommonLog := strings.Replace(caddyLog.CommonLog, search, replace, 1)

		toPrint := fmt.Sprintf("%s:%s \"%s\" \"%s\"\n", caddyLog.Request.Host, myCommonLog, referer, userAgent)

		fmt.Fprintf(os.Stdout, toPrint)
		fmt.Fprintf(os.Stderr, " %s matched %s: %s", contentType, contentTypeWhitelistForDebugLog, toPrint)
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

	return config
}
