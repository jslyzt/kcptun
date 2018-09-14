GOFLAGS=-ldflags "-X \"main.version=`git rev-list HEAD | wc -l`\" -X \"main.commitID=`git rev-list HEAD | head -1`\""
BLDDIR = build
EXEPAD =

ifeq "$(GOOS)" "windows"
	EXEPAD=.exe
endif

all:
	mkdir -p $(BLDDIR)
	cd server; go build ${GOFLAGS} -o ../$(BLDDIR)/server$(EXEPAD)
	cd client; go build ${GOFLAGS} -o ../$(BLDDIR)/client$(EXEPAD)

clean:
	rm -fr $(BLDDIR)
