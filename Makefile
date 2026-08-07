.PHONY: build frontend backend dev run clean

frontend:
	cd web && npm install && npm run build

backend:
	go build -o data777 ./cmd/data777

build: frontend backend

dev:
	@echo "Run these in two terminals:"
	@echo "  go run ./cmd/data777 --data-dir ./devdata"
	@echo "  cd web && npm run dev"

run: backend
	./data777 --data-dir ./devdata

clean:
	rm -rf data777 web/node_modules internal/server/webdist/assets
