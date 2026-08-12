.PHONY: test
test:
	$(MAKE) -C orlop test
	$(MAKE) -C platform-api test
	$(MAKE) -C controllers test
