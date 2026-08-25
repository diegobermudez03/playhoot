Package `monitoring` exposes general monitoring capabilites, like metrics, alerts, etc.

Right now is a single pkg in the monolith, all the domains can use it, as we expect everything to be running in same process.

But eventually, when we move services out, each domain should have a copy of this pkg, which then will have a specific implementation for its service metrics and stuff.
