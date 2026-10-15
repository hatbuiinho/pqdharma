.PHONY: serve

# Forward server arguments with make serve ARGS="-addr :8081".
serve:
	air -c .air.toml $(if $(ARGS),-- $(ARGS))
