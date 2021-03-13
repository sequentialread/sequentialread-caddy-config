
# sequentialread-caddy-config

This application talks to the docker socket to get info about containers and then
generates a Caddy config, which it posts to Caddy 2 HTTP server (by Matt Holt, of Let's Encrypt fame).

Yes I realize https://github.com/lucaslorentz/caddy-docker-proxy already does this :P 

I am making my own instead for a couple reasons: 

1. I'm going to give it access to the docker socket so I'd prefer to know the code well.
2. I already created most of it for the [ingress controller part of rootsystem](https://git.sequentialread.com/forest/rootsystem/src/master/automation/ingressController.go). 


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