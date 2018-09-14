GOFLAGS=-ldflags "-X \"main.version=`git rev-list HEAD | wc -l`\" -X \"main.commitID=`git rev-list HEAD | head -1`\""
BLDDIR = build

all:
	mkdir -p $(BLDDIR)
	cd server; go build ${GOFLAGS} -o ../$(BLDDIR)/server main.go
	cd client; go build ${GOFLAGS} -o ../$(BLDDIR)/client main.go

clean:
	rm -fr $(BLDDIR)
