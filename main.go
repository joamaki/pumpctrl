package main

import (
    "fmt"
    "log"
    "github.com/stianeikeland/go-rpio/v4"
    "net/http"
    "time"
    "os/exec"
    "os"
    "io"
    "sync"
)

var (
	pumpPin = rpio.Pin(26)
	ledAPin = rpio.Pin(5)
	ledBPin = rpio.Pin(6)
	switchPin = rpio.Pin(22)
	trigPin = rpio.Pin(19)
	echoPin = rpio.Pin(13)

	pumpCount = 0
	lastPumped = time.Time{}
	pumpTime = time.Second*25
	pumping = false
	lock sync.Mutex
)

func pumpOff() {
	log.Printf("Pump off\n")
	pumpPin.High()
	ledAPin.High()
	ledBPin.Low()
}

func pumpOn() {
	log.Printf("Pump on\n")
	pumpPin.Low()
	ledAPin.Low()
	ledBPin.High()
}

func goPump() {
	lock.Lock()
	defer lock.Unlock()

	if pumping {
        	return
	}
	pumping = true
	lastPumped = time.Now()
	pumpCount++

        pumpOn()
        go func() {
		time.Sleep(pumpTime)
		pumpOff()

		lock.Lock()
		pumping = false
		lock.Unlock()
        }()
}

func togglePump() bool {
    	goPump()
    	return pumping
    	/*
	pumpPin.Toggle()
	ledAPin.Toggle()
	ledBPin.Toggle()

	s := pumpPin.Read()
	log.Printf("Toggling pump, new state: %v\n", s)
	return s == rpio.Low
	*/
}

func handleTogglePump(w http.ResponseWriter, r *http.Request) {
    	w.Header().Add("Content-Type", "application/json")
	pumping := togglePump()
	fmt.Fprintf(w, `{"pumping": %v}`, pumping)
}

func handleStats(w http.ResponseWriter, r *http.Request) {
    	w.Header().Add("Content-Type", "application/json")
	fmt.Fprintf(w, `{"lastPump": %q, "pumpCount": %d}`, lastPumped, pumpCount)
}

func handleDistance(w http.ResponseWriter, r *http.Request) {
	dist, timeout := measureDistance()
    	w.Header().Add("Content-Type", "application/json")
	fmt.Fprintf(w, `{"distance": %f, "timeout": %v}`, dist, timeout)
}

func handleCaptureImage(w http.ResponseWriter, r *http.Request) {
    	log.Printf("Capturing image...\n")

	cmd := exec.Command("libcamera-jpeg",
	                    "--tuning-file",
	                    "/usr/share/libcamera/ipa/raspberrypi/imx219_noir.json",
	                    "-o", "/tmp/image.jpeg")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()

	w.Header().Add("Content-Type", "image/jpeg")
	f, err := os.Open("/tmp/image.jpeg")
	if err != nil {
		log.Printf("Failed to open /tmp/image.jpeg: %s\n", err)
		w.WriteHeader(500)
		return
	}
	defer f.Close()
	io.Copy(w, f)
}

// Distance is a distance measured in centimeters
type Distance float64

func measureDistance() (Distance, bool) {
        var timedOut = false
	go func() {
    		time.Sleep(10*time.Millisecond)
    		timedOut = true
	}()

	// Trigger the measurement
    	trigPin.High()
    	time.Sleep(time.Microsecond)
    	trigPin.Low()

	// Wait for echo to go low
	for !timedOut && echoPin.Read() == rpio.Low {}
	t0 := time.Now()

	for !timedOut && echoPin.Read() == rpio.High {}
	t1 := time.Now()

	distance := (float64(t1.Sub(t0))/float64(time.Second) * 34300.0) / 2.0

	log.Printf("Measured distance of %fcm (timeout: %v)\n", distance, timedOut)
	return Distance(distance), timedOut
}

func main() {
	if err := rpio.Open(); err != nil {
        	log.Fatal(err)
	}

	pumpPin.Mode(rpio.Output)
	ledAPin.Mode(rpio.Output)
	ledBPin.Mode(rpio.Output)
	switchPin.Mode(rpio.Input)

	ledAPin.High()
	ledBPin.Low()

	trigPin.Mode(rpio.Output)
	echoPin.Mode(rpio.Input)

	dist, tout := measureDistance()
	fmt.Printf("dist=%v, tout=%v\n", dist, tout)

	pumpOff()

	go func() {
    		state := switchPin.Read()
    		for range(time.Tick(time.Millisecond*500)) {
        		newState := switchPin.Read()
        		if state != newState {
            			state = newState
            			if state == rpio.High {
                			pumpOn()
            			} else {
                			pumpOff()
            			}
        		}
    		}
	}()

	go func() {
    		for {
        		time.Sleep(time.Hour*48)
            		goPump()
    		}
	}()

	http.HandleFunc("/api/pump", handleTogglePump)
	http.HandleFunc("/api/distance", handleDistance)
	http.HandleFunc("/api/capture", handleCaptureImage)
	http.HandleFunc("/api/stats", handleStats)
	http.Handle("/", http.FileServer(http.Dir("/home/pi/static/")))
	
	log.Fatal(http.ListenAndServe(":8080", nil))
}
