package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// IPInfo JSON response from IPInfo
type IPInfo struct {
	City     string `json:"city"`
	Region   string `json:"region"`
	Country  string `json:"country"`
	Loc      string `json:"loc"`
	Org      string `json:"org"`
	Timezone string `json:"timezone"`
}

// getIPAddress extracts the client's IP address from the request.
func getIPAddress(r *http.Request) string {
	if xForwardedFor := r.Header.Get("X-Forwarded-For"); xForwardedFor != "" {
		parts := strings.Split(xForwardedFor, ",")
		return strings.TrimSpace(parts[0])
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		log.Printf("Error getting IP: %v", err)
		return ""
	}
	return ip
}

// getIPInfo calls the IPInfo API to get information about the IP address.
func getIPInfo(ip string) (*IPInfo, error) {

	url := fmt.Sprintf("http://ipinfo.io/%s?token=35c09591be32a1", ip)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var ipInfo IPInfo
	err = json.Unmarshal(body, &ipInfo)
	if err != nil {
		return nil, err
	}
	return &ipInfo, nil
}

// isPrivateIP checks if an IP address is a private address.
func isPrivateIP(ip string) bool {
	privateIPBlocks := []*net.IPNet{
		mustParseCIDR("10.0.0.0/8"),
		mustParseCIDR("172.16.0.0/12"),
		mustParseCIDR("192.168.0.0/16"),
		mustParseCIDR("127.0.0.0/8"),
	}

	parsedIP := net.ParseIP(ip)
	for _, block := range privateIPBlocks {
		if block.Contains(parsedIP) {
			return true
		}
	}
	return false
}

func mustParseCIDR(cidr string) *net.IPNet {
	_, block, _ := net.ParseCIDR(cidr)
	return block
}

// ipHandler handles the incoming HTTP request and writes the client's IP info.
func ipHandler(w http.ResponseWriter, r *http.Request) {
	ipAddress := getIPAddress(r)
	log.Printf("Received request from: %s", ipAddress)

	if isPrivateIP(ipAddress) {
		fmt.Fprintf(w, "Your IP address is: %s\nLocation: N/A (private IP)\nTimezone: N/A (private IP)", ipAddress)
		return
	}

	ipInfo, err := getIPInfo(ipAddress)
	if err != nil {
		fmt.Fprintf(w, "Error getting IP info: %s", err)
		return
	}

	response := fmt.Sprintf("Your IP address is: %s\nLocation: %s, %s, %s\nTimezone: %s",
		ipAddress, ipInfo.City, ipInfo.Region, ipInfo.Country, ipInfo.Timezone)
	fmt.Fprint(w, response)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default port if not specified
	}

	server := &http.Server{Addr: ":" + port, Handler: nil}

	// Handle routes
	http.HandleFunc("/", ipHandler)

	// Start server in a goroutine so that it doesn't block
	go func() {
		fmt.Println("Starting server on :", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exiting")
}
