# Use the official Golang image as a base image
FROM golang:1.22

# Set the current working directory inside the container
WORKDIR /app

# Copy the source code into the container
COPY . .

# Build the Go app
RUN go build -o http2_client .

# Command to run the executable
CMD ["./http2_client"]
