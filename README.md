
# sequentialread-caddy-config

This application talks to the docker socket to get info about containers and then
generates a caddy config, which it posts to Caddy 2 HTTP server by Let's Encrypt.

Yes I realize https://github.com/lucaslorentz/caddy-docker-proxy already does this :P 

I am making my own instead for a couple reasons: 

1. I'm going to give it access to the docker socket so I'd prefer to know the code well.
2. I already created most of it for the [ingress controller part of rootsystem](https://git.sequentialread.com/forest/rootsystem/src/master/automation/ingressController.go). 

