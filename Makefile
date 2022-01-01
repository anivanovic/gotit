
run: gotit
	./bin/gotit

clean:
	rm -rf bin/*

gotit:
	go build -o bin/gotit cmd/gotit/main.go

bencode:
	go run cmd/bencode/main.go -file=$(file)

.PHONY: clean run 