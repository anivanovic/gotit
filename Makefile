
run: gotit
	./bin/gotit

clean:
	rm -rf bin/*

gotit:
	go build -o bin/gotit cmd/gotit.go

.PHONY: clean run