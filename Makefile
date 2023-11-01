NAME = containerd

GOOS = linux
GOARCH = amd64

PROTO_DIR = ./proto
PROTO_OUTPUT_DIR = ./pb
OUTPUT_DIR = ./build

all: $(NAME)

$(NAME): $(OUTPUT_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o ${OUTPUT_DIR}/${NAME}

proto: $(PROTO_OUTPUT_DIR)
	protoc --proto_path=$(PROTO_DIR) $(PROTO_DIR)/$(NAME).proto --go_out=$(PROTO_OUTPUT_DIR) --go_opt=paths=source_relative --go-grpc_out=$(PROTO_OUTPUT_DIR) --go-grpc_opt=paths=source_relative

pclean:
	rm -rf $(PROTO_OUTPUT_DIR)

fclean:
	rm -rf $(OUTPUT_DIR)

re: fclean all

$(PROTO_OUTPUT_DIR):
	@mkdir -p $@

$(OUTPUT_DIR):
	@mkdir -p $@
