.PHONY: macos

macos:
	@set -eu; \
	test "$$(uname -s)" = Darwin || { echo "Le build Pupitre.app doit être lancé sur macOS." >&2; exit 1; }; \
	case "$$(uname -m)" in \
		arm64) arch=arm64 ;; \
		x86_64) arch=amd64 ;; \
		*) echo "Architecture macOS non prise en charge." >&2; exit 1 ;; \
	esac; \
	mkdir -p dist; \
	stage=$$(mktemp -d dist/.pupitre.XXXXXX); \
	trap 'rm -rf "$$stage"' EXIT; \
	bundle="$$stage/Pupitre.app"; \
	mkdir -p "$$bundle/Contents/MacOS" "$$bundle/Contents/Resources"; \
	GOOS=darwin GOARCH="$$arch" CGO_ENABLED=1 go build -o "$$bundle/Contents/MacOS/Pupitre" .; \
	cp macos/Info.plist "$$bundle/Contents/Info.plist"; \
	cp macos/Pupitre.icns "$$bundle/Contents/Resources/Pupitre.icns"; \
	codesign --force --sign - "$$bundle"; \
	codesign --verify --deep --strict "$$bundle"; \
	rm -rf dist/Pupitre.app; \
	mv "$$bundle" dist/Pupitre.app; \
	echo "Application créée : dist/Pupitre.app"
