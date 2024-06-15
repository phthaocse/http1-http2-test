# Use the official Golang image as a base image
FROM golang:1.22

# Set the current working directory inside the container
WORKDIR /app

# Copy the source code into the container
COPY . .

# Copy SSL certificates
COPY server.crt server.crt
COPY server.key server.key

# Build the Go app
RUN go build -o server .

# Command to run the executable
CMD ["./server"]
