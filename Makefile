.PHONY: build test vet install clean

build:
	go build -o qs.exe .

test:
	go test ./...

vet:
	go vet ./...

install: build
	powershell -Command "& .\install.ps1"

clean:
	-rm -f qs.exe
