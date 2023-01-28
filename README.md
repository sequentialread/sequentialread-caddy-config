
# sequentialread-caddy-config

This application talks to the docker socket to get info about containers and then
generates a Caddy config, which it posts to Caddy 2 HTTP server. 

This is similar to https://traefik.io/ or https://github.com/nginx-proxy/nginx-proxy

Yes I realize https://github.com/lucaslorentz/caddy-docker-proxy already does this :P 

I am making my own instead for a couple reasons: 

1. I don't like the template-based solutions because they are harder to debug. One typically cannot put breakpoints or print statements inside a large complicated template file. 
2. I had already written code that generates Caddy configs for some of my other projects. This is the code that eventually became  [greenhouse-daemon](https://git.sequentialread.com/sqr/greenhouse-daemon/src/commit/c563be03d35ee5d56d040ae7a3a1ca455bb79d92/config_service.go). 


Example docker labels to configure a container to be served publically:

```
	sequentialread-80-public-port: 443
	sequentialread-80-public-protocol: https
	sequentialread-80-public-hostnames: "example.com,www.example.com"
	sequentialread-80-container-protocol: http
```



# how to generate favicon for ghost

```
sudo apt install icoutils
icotool -c -o test.ico sequentialread_favicon.png logo48.png logo70.png logo128.png 
```