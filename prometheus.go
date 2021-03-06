// Main program logic
// Relies on utils, structs, and gpio libraries.
package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gilgameshskytrooper/prometheusweb/gpio"
	"github.com/gilgameshskytrooper/prometheusweb/structs"
	"github.com/gilgameshskytrooper/prometheusweb/utils"
	"gopkg.in/go-playground/colors.v1"
	"github.com/robfig/cron"

)

var (
	//Alarm object from the structs.Alarm{} struct
	Alarm1 = structs.Alarm{}
	//Alarm object from the structs.Alarm{} struct
	Alarm2 = structs.Alarm{}
	//Alarm object from the structs.Alarm{} struct
	Alarm3 = structs.Alarm{}
	//Alarm object from the structs.Alarm{} struct
	Alarm4 = structs.Alarm{}
	// Bool to check if the Nixie Clock was found.
	foundNixie bool
	// Bool to keep track of whether the user wants to receive emails when the Pi's IP changes.
	// (If using DDNS, this is not necessary as the DDNS client will automatically update the relevant IP, and it will always be reachable with the same command)
	EnableEmail bool
	// The email address to send the IP change notification email to
	Email string
	// Bool to keep track of whether shairport-sync is installed
	shairportInstalled bool
	// Alarm sound filename
	Soundname       string
	CustomSoundCard bool
)

//Function to check whether the alarm has been running for more than 10 minutes
func OverTenMinutes(alarmtime string) bool {
	year, month, day := time.Now().Date()
	var hour int
	var minutes int
	if string([]rune(alarmtime)[0]) == "0" {
		hour, _ = strconv.Atoi(string([]rune(alarmtime)[1:2]))
	} else {
		hour, _ = strconv.Atoi(string([]rune(alarmtime)[0:2]))
	}

	if string([]rune(alarmtime)[3]) == "0" {
		minutes, _ = strconv.Atoi(string([]rune(alarmtime)[4]))
	} else {
		minutes, _ = strconv.Atoi(string([]rune(alarmtime)[3:]))
	}

	timecurrent := time.Now()
	difference := timecurrent.Minute() - time.Date(int(year), month, int(day), hour, minutes, 0, 0, time.Local).Minute()
	if difference == 10 {
		return true
	} else {
		return false
	}
}

//Handle the AJAX file upload from the client. Deletes the old file, and saves the new name of the sound from the header.Filename
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	//Delete the old alarm sound via shell command process rm public/assets/sound_name.extension
      err := os.Remove(utils.Pwd()+"/public/assets/"+Soundname)

      if err != nil {
          fmt.Println(err)
          return
      }
	file, header, err := r.FormFile("audio")
	//Set the Soundname attribute to the new soundname
	Soundname = header.Filename

	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	defer file.Close()

	//Write the soundname to ./public/assets/sound_name.extension
	out, err1 := os.Create(utils.Pwd() + "/public/assets/" + header.Filename)

	if err1 != nil {
		fmt.Fprintf(w, "Unable to upload the file")
	}
	defer out.Close()
	_, err2 := io.Copy(out, file)
	if err2 != nil {
		fmt.Fprintln(w, err)
	}
	fmt.Fprintf(w, "File uploaded successfully :")

    files, err := ioutil.ReadDir(utils.Pwd()+"/public/assets")
    if err != nil {
        log.Fatal(err)
    }
	// If a new file got uploaded, make sure this gets reflected in the program var Soundname and also write out the new name into ./public/json/trackinfo
	Soundname = strings.TrimSpace(files[0].Name())
	d1 := []byte(Soundname)
	errrrrrrrrr := ioutil.WriteFile(utils.Pwd()+"/public/json/trackinfo", d1, 0644)
	if errrrrrrrrr != nil {
		fmt.Println(errrrrrrrrr)
	}

	fmt.Fprintf(w, header.Filename)
}

//Initialization sequence
// 1. Grab the persistent alarm information from the alarms.json file
// 2. Use this data to store legitimate values of time, vibration, and sound into the 4 struct.Alarm objects
// 3. Grab the name of the current sound file via an "ls" shell command, and save it to the global variable Sound, and then write that information into ./public/json/trackinfo
// 4. Get the email of user to be used in the CheckIPChange() function
// 5. Get the persistent data of whether or not the user wants Prometheus to send emails when the IP changes. (Note, this is probably alot easier done through a dynamic DNS service which runs a background program to constanty check the IP of the Pi, updates a domain name server, and then you can access that as a link such as myclockname.ddns.net:3000
func init() {
	//Save the JSON alarms configurations into the mold
	jsondata := structs.GetRawJson(utils.Pwd() + "/public/json/alarms.json")
	Alarm1.InitializeAlarms(jsondata, 0)
	Alarm2.InitializeAlarms(jsondata, 1)
	Alarm3.InitializeAlarms(jsondata, 2)
	Alarm4.InitializeAlarms(jsondata, 3)

	files, errdir := ioutil.ReadDir(utils.Pwd()+"/public/assets")
	if errdir != nil {
		log.Println(errdir)
	}

	Soundname = strings.TrimSpace(files[0].Name())
	d1 := []byte(Soundname)
	errrrrrrrrr := ioutil.WriteFile(utils.Pwd()+"/public/json/trackinfo", d1, 0644)
	if errrrrrrrrr != nil {
		fmt.Println(errrrrrrrrr)
	}
	Email = utils.GetEmail()
	EnableEmail = utils.GetEnableEmail()
	CustomSoundCard = utils.UseCustomSoundCard()
}

// Main function
// Runs the cron job (checking once a minute at exactly the point when second is 00) to check if the current time matches the user supplied alarm time configuration, and then runs the alarm if an enabled alarm matches the time
// Runs a separate cron job (once every second) to send the current time as a string to the Nixie Clock through serial USB
// Also, main contains all the http HandleFunc's to deal with GET '/', POST '/time', POST '/sound', POST '/vibration', POST '/snooze', POST '/enableemail', POST '/newemail'
func main() {

	// Ensure any previous incarnations of shairport-sync gets killed.
	// if no previous process exists, KillShairportSync() automatically handles this.
	shairportInstalled = utils.CheckShairportSyncInstalled()
	if shairportInstalled {
		fmt.Println("shairport-sync", "-d")
	}

	// Initialize all 4 instances of alarm clocks
	// Create function that updates clock once a minute (used to see if any times match up)
	t := time.Now()
	currenttime := t.Format("15:04")
	c := cron.New()

	//Run the following once a minute
	//Check all 4 alarms to see if the current time matches any configurations
	c.AddFunc("0 * * * * *", func() {
		breaktime := false
		duration := time.Second * 3
		t = time.Now()
		currenttime = t.Format("15:04")
		if EnableEmail {
			utils.CheckIPChange()
		}

		if Alarm1.Alarmtime == currenttime {

			Alarm1.CurrentlyRunning = true

			if Alarm1.Sound && Alarm1.Vibration {

				if shairportInstalled {
					utils.KillShairportSync()
				}

				if CustomSoundCard {
					fmt.Println("Start Sound")

					for {
						gpio.VibOn()
						for i := 1; i <= 50; i++ {
							time.Sleep(time.Millisecond * 50)
							if !Alarm1.CurrentlyRunning {
								breaktime = true
								break
							}
						}
						if breaktime {

							gpio.VibOff()
							fmt.Println("Kill Sound")
							breaktime = false
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else if OverTenMinutes(Alarm1.Alarmtime) {
							Alarm1.CurrentlyRunning = false
							gpio.VibOff()
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else {
							gpio.VibOff()
							time.Sleep(duration)
						}

					}
				} else {
					fmt.Println("Start Sound")
					for {
						gpio.VibOn()
						for i := 1; i <= 50; i++ {
							time.Sleep(time.Millisecond * 50)
							if !Alarm1.CurrentlyRunning {
								breaktime = true
								break
							}
						}
						if breaktime {

							gpio.VibOff()
							fmt.Println("Kill Sound")
							breaktime = false
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else if OverTenMinutes(Alarm1.Alarmtime) {
							Alarm1.CurrentlyRunning = false
							gpio.VibOff()
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else {
							gpio.VibOff()
							time.Sleep(duration)
						}

					}
				}

			} else if Alarm1.Sound && !Alarm1.Vibration {

				if shairportInstalled {
					utils.KillShairportSync()
				}
				if CustomSoundCard {
					fmt.Println("Start Sound")
					for {
						time.Sleep(time.Second * 1)
						if !Alarm1.CurrentlyRunning {
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else if OverTenMinutes(Alarm1.Alarmtime) {
							Alarm1.CurrentlyRunning = false
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						}
					}
				} else {
					fmt.Println("Start Sound")
					for {
						time.Sleep(time.Second * 1)
						if !Alarm1.CurrentlyRunning {
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else if OverTenMinutes(Alarm1.Alarmtime) {
							Alarm1.CurrentlyRunning = false
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						}
					}
				}

			} else if !Alarm1.Sound && Alarm1.Vibration {
				for {
					gpio.VibOn()
					for i := 1; i <= 50; i++ {
						time.Sleep(time.Millisecond * 50)
						if !Alarm1.CurrentlyRunning {
							breaktime = true
							break
						}
					}
					if breaktime {
						gpio.VibOff()
						breaktime = false
						break
					} else if OverTenMinutes(Alarm1.Alarmtime) {
						Alarm1.CurrentlyRunning = false
						gpio.VibOff()
					} else {
						gpio.VibOff()
						time.Sleep(duration)
					}
				}
			} else {
				Alarm1.CurrentlyRunning = false
			}

		} else if Alarm2.Alarmtime == currenttime {
			// Check if there is network connectivity (if not, then restart network interfaces)
			Alarm2.CurrentlyRunning = true
			if Alarm2.Sound && Alarm2.Vibration {

				if shairportInstalled {
					utils.KillShairportSync()
				}

				if CustomSoundCard {
					fmt.Println("Start Sound")

					for {
						gpio.VibOn()
						for i := 1; i <= 50; i++ {
							time.Sleep(time.Millisecond * 50)
							if !Alarm2.CurrentlyRunning {
								breaktime = true
								break
							}
						}
						if breaktime {
							gpio.VibOff()
							fmt.Println("Kill Sound")
							breaktime = false
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else if OverTenMinutes(Alarm2.Alarmtime) {
							Alarm2.CurrentlyRunning = false
							gpio.VibOff()
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else {
							gpio.VibOff()
							time.Sleep(duration)
						}

					}
				} else {
					fmt.Println("Start Sound")
					for {
						gpio.VibOn()
						for i := 1; i <= 50; i++ {
							time.Sleep(time.Millisecond * 50)
							if !Alarm2.CurrentlyRunning {
								breaktime = true
								break
							}
						}
						if breaktime {
							gpio.VibOff()
							fmt.Println("Kill Sound")
							breaktime = false
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else if OverTenMinutes(Alarm2.Alarmtime) {
							Alarm2.CurrentlyRunning = false
							gpio.VibOff()
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else {
							gpio.VibOff()
							time.Sleep(duration)
						}

					}
				}

			} else if Alarm2.Sound && !Alarm2.Vibration {

				if shairportInstalled {
					utils.KillShairportSync()
				}

				if CustomSoundCard {
					fmt.Println("Start Sound")

					for {
						time.Sleep(time.Second * 1)
						if !Alarm2.CurrentlyRunning {
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else if OverTenMinutes(Alarm2.Alarmtime) {
							Alarm2.CurrentlyRunning = false
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						}
					}

				} else {
					fmt.Println("Start Sound")
					for {
						time.Sleep(time.Second * 1)
						if !Alarm2.CurrentlyRunning {
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else if OverTenMinutes(Alarm2.Alarmtime) {
							Alarm2.CurrentlyRunning = false
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						}
					}
				}

			} else if !Alarm2.Sound && Alarm2.Vibration {
				for {
					gpio.VibOn()
					for i := 1; i <= 50; i++ {
						time.Sleep(time.Millisecond * 50)
						if !Alarm2.CurrentlyRunning {
							breaktime = true
							break
						}
					}
					if breaktime {
						gpio.VibOff()
						breaktime = false
						break
					} else if OverTenMinutes(Alarm2.Alarmtime) {
						Alarm2.CurrentlyRunning = false
						gpio.VibOff()
					} else {
						gpio.VibOff()
						time.Sleep(duration)
					}
				}
			} else {
				Alarm2.CurrentlyRunning = false
			}

		} else if Alarm3.Alarmtime == currenttime {
			// Check if there is network connectivity (if not, then restart network interfaces)
			Alarm3.CurrentlyRunning = true
			if Alarm3.Sound && Alarm3.Vibration {

				if shairportInstalled {
					utils.KillShairportSync()
				}

				if CustomSoundCard {
					fmt.Println("Start Sound")

					for {
						gpio.VibOn()
						for i := 1; i <= 50; i++ {
							time.Sleep(time.Millisecond * 50)
							if !Alarm3.CurrentlyRunning {
								breaktime = true
								break
							}
						}
						if breaktime {
							gpio.VibOff()
							fmt.Println("Kill Sound")
							breaktime = false
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else if OverTenMinutes(Alarm3.Alarmtime) {
							Alarm3.CurrentlyRunning = false
							gpio.VibOff()
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else {
							gpio.VibOff()
							time.Sleep(duration)
						}

					}
				} else {
					fmt.Println("Start Sound")

					for {
						gpio.VibOn()
						for i := 1; i <= 50; i++ {
							time.Sleep(time.Millisecond * 50)
							if !Alarm3.CurrentlyRunning {
								breaktime = true
								break
							}
						}
						if breaktime {
							gpio.VibOff()
							fmt.Println("Kill Sound")
							breaktime = false
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else if OverTenMinutes(Alarm3.Alarmtime) {
							Alarm3.CurrentlyRunning = false
							gpio.VibOff()
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else {
							gpio.VibOff()
							time.Sleep(duration)
						}

					}
				}

			} else if Alarm3.Sound && !Alarm3.Vibration {

				if shairportInstalled {
					utils.KillShairportSync()
				}

				if CustomSoundCard {
					fmt.Println("Start Sound")

					for {
						time.Sleep(time.Second * 1)
						if !Alarm3.CurrentlyRunning {
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else if OverTenMinutes(Alarm3.Alarmtime) {
							Alarm3.CurrentlyRunning = false
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						}
					}

				} else {
					fmt.Println("Start Sound")

					for {
						time.Sleep(time.Second * 1)
						if !Alarm3.CurrentlyRunning {
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else if OverTenMinutes(Alarm3.Alarmtime) {
							Alarm3.CurrentlyRunning = false
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						}
					}

				}

			} else if !Alarm3.Sound && Alarm3.Vibration {
				for {
					gpio.VibOn()
					for i := 1; i <= 50; i++ {
						time.Sleep(time.Millisecond * 50)
						if !Alarm3.CurrentlyRunning {
							breaktime = true
							break
						}
					}
					if breaktime {
						gpio.VibOff()
						breaktime = false
						break
					} else if OverTenMinutes(Alarm3.Alarmtime) {
						Alarm3.CurrentlyRunning = false
						gpio.VibOff()
					} else {
						gpio.VibOff()
						time.Sleep(duration)
					}
				}
			} else {
				Alarm3.CurrentlyRunning = false
			}

		} else if Alarm4.Alarmtime == currenttime {
			// Check if there is network connectivity (if not, then restart network interfaces)
			Alarm4.CurrentlyRunning = true
			if Alarm4.Sound && Alarm4.Vibration {

				if shairportInstalled {
					utils.KillShairportSync()
				}

				if CustomSoundCard {
					fmt.Println("Start Sound")

					for {
						gpio.VibOn()
						for i := 1; i <= 50; i++ {
							time.Sleep(time.Millisecond * 50)
							if !Alarm4.CurrentlyRunning {
								breaktime = true
								break
							}
						}
						if breaktime {
							gpio.VibOff()
							fmt.Println("Kill Sound")
							breaktime = false
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else if OverTenMinutes(Alarm4.Alarmtime) {
							Alarm4.CurrentlyRunning = false
							gpio.VibOff()
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else {
							gpio.VibOff()
							time.Sleep(duration)
						}

					}

				} else {
					fmt.Println("Start Sound")

					for {
						gpio.VibOn()
						for i := 1; i <= 50; i++ {
							time.Sleep(time.Millisecond * 50)
							if !Alarm4.CurrentlyRunning {
								breaktime = true
								break
							}
						}
						if breaktime {
							gpio.VibOff()
							fmt.Println("Kill Sound")
							breaktime = false
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else if OverTenMinutes(Alarm4.Alarmtime) {
							Alarm4.CurrentlyRunning = false
							gpio.VibOff()
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else {
							gpio.VibOff()
							time.Sleep(duration)
						}

					}

				}

			} else if Alarm4.Sound && !Alarm4.Vibration {

				if shairportInstalled {
					utils.KillShairportSync()
				}

				if CustomSoundCard {
					fmt.Println("Start Sound")

					for {
						time.Sleep(time.Second * 1)
						if !Alarm4.CurrentlyRunning {
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else if OverTenMinutes(Alarm4.Alarmtime) {
							Alarm4.CurrentlyRunning = false
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						}
					}

				} else {
					fmt.Println("Start Sound")

					for {
						time.Sleep(time.Second * 1)
						if !Alarm4.CurrentlyRunning {
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						} else if OverTenMinutes(Alarm4.Alarmtime) {
							Alarm4.CurrentlyRunning = false
							fmt.Println("Kill Sound")
							if shairportInstalled {
								fmt.Println("shairport-sync", "-d")
							}
							break
						}
					}

				}

			} else if !Alarm4.Sound && Alarm4.Vibration {
				for {
					gpio.VibOn()
					for i := 1; i <= 50; i++ {
						time.Sleep(time.Millisecond * 50)
						if !Alarm4.CurrentlyRunning {
							breaktime = true
							break
						}
					}
					if breaktime {
						gpio.VibOff()
						breaktime = false
						break
					} else if OverTenMinutes(Alarm4.Alarmtime) {
						Alarm4.CurrentlyRunning = false
						gpio.VibOff()
					} else {
						gpio.VibOff()
						time.Sleep(duration)
					}
				}
			} else {
				Alarm4.CurrentlyRunning = false
			}
		}
	})
	c.Start()

	// Server index.html under ./public/index.html
	fs := http.FileServer(http.Dir(utils.Pwd() + "/public"))
	http.Handle("/", fs)

	//Handle the AJAX post call to submit a new time for a certain alarm
	http.HandleFunc("/time", func(w http.ResponseWriter, r *http.Request) {
		erawr := r.ParseForm()
		if erawr != nil {
			fmt.Println("submit time failed")
			os.Exit(1)
		}
		name := r.FormValue("name")
		time := r.FormValue("value")
		//Check to see which alarm the user is actually trying to modify, and modify the correct internally stored time
		if name == "alarm1" {
			Alarm1.Alarmtime = time
			Alarm1.CurrentlyRunning = false
		} else if name == "alarm2" {
			Alarm2.Alarmtime = time
			Alarm2.CurrentlyRunning = false
		} else if name == "alarm3" {
			Alarm3.Alarmtime = time
			Alarm3.CurrentlyRunning = false
		} else if name == "alarm4" {
			Alarm4.Alarmtime = time
			Alarm4.CurrentlyRunning = false
		}
		//Write back the alarm data back to the ./public/json/alarms.json so Prometheus could retreive the data after a restart
		utils.WriteBackJson(Alarm1, Alarm2, Alarm3, Alarm4, utils.Pwd()+"/public/json/alarms.json")
	})

	http.HandleFunc("/sound", func(w http.ResponseWriter, r *http.Request) {
		erawr := r.ParseForm()
		if erawr != nil {
			fmt.Println("submit sound failed")
			os.Exit(1)
		}
		name := r.FormValue("name")
		sound := r.FormValue("value")
		var boolsound bool
		if sound == "on" {
			boolsound = true
		} else {
			boolsound = false
		}

		//Check to see which alarm the user is actually trying to modify, and modify the correct internally stored sound
		if name == "alarm1" {
			Alarm1.Sound = boolsound
			Alarm1.CurrentlyRunning = false
		} else if name == "alarm2" {
			Alarm2.Sound = boolsound
			Alarm2.CurrentlyRunning = false
		} else if name == "alarm3" {
			Alarm3.Sound = boolsound
			Alarm3.CurrentlyRunning = false
		} else if name == "alarm4" {
			Alarm4.Sound = boolsound
			Alarm4.CurrentlyRunning = false
		}
		//Write back the alarm data back to the ./public/json/alarms.json so Prometheus could retreive the data after a restart
		utils.WriteBackJson(Alarm1, Alarm2, Alarm3, Alarm4, utils.Pwd()+"/public/json/alarms.json")
	})

	http.HandleFunc("/vibration", func(w http.ResponseWriter, r *http.Request) {
		erawr := r.ParseForm()
		if erawr != nil {
			fmt.Println("submit vibration failed")
			os.Exit(1)
		}
		name := r.FormValue("name")
		vibration := r.FormValue("value")
		var boolvibration bool
		if vibration == "on" {
			boolvibration = true
		} else {
			boolvibration = false
		}
		//Check to see which alarm the user is actually trying to modify, and modify the correct internally stored vibration
		if name == "alarm1" {
			Alarm1.Vibration = boolvibration
			Alarm1.CurrentlyRunning = false
		} else if name == "alarm2" {
			Alarm2.Vibration = boolvibration
			Alarm2.CurrentlyRunning = false
		} else if name == "alarm3" {
			Alarm3.Vibration = boolvibration
			Alarm3.CurrentlyRunning = false
		} else if name == "alarm4" {
			Alarm4.Vibration = boolvibration
			Alarm4.CurrentlyRunning = false
		}
		//Write back the alarm data back to the ./public/json/alarms.json so Prometheus could retreive the data after a restart
		utils.WriteBackJson(Alarm1, Alarm2, Alarm3, Alarm4, utils.Pwd()+"/public/json/alarms.json")
	})

	http.HandleFunc("/snooze", func(w http.ResponseWriter, r *http.Request) {
		//Using the AddTime() function, add 10 minutes to the currently running alarm, turn off the currently running alarm, and write back the correct configuration back to ./public/json/alarms.json
		if Alarm1.CurrentlyRunning {
			Alarm1.CurrentlyRunning = false
			Alarm1.AddTime(Alarm1.Alarmtime, "m", 10)
			utils.WriteBackJson(Alarm1, Alarm2, Alarm3, Alarm4, utils.Pwd()+"/public/json/alarms.json")
		} else if Alarm2.CurrentlyRunning {
			Alarm2.CurrentlyRunning = false
			Alarm2.AddTime(Alarm2.Alarmtime, "m", 10)
			utils.WriteBackJson(Alarm1, Alarm2, Alarm3, Alarm4, utils.Pwd()+"/public/json/alarms.json")
		} else if Alarm3.CurrentlyRunning {
			Alarm3.CurrentlyRunning = false
			Alarm3.AddTime(Alarm3.Alarmtime, "m", 10)
			utils.WriteBackJson(Alarm1, Alarm2, Alarm3, Alarm4, utils.Pwd()+"/public/json/alarms.json")
		} else if Alarm4.CurrentlyRunning {
			Alarm4.CurrentlyRunning = false
			Alarm4.AddTime(Alarm4.Alarmtime, "m", 10)
			utils.WriteBackJson(Alarm1, Alarm2, Alarm3, Alarm4, utils.Pwd()+"/public/json/alarms.json")
		}
		http.Redirect(w, r, "/", 301)
	})

	http.HandleFunc("/enableemail", func(w http.ResponseWriter, r *http.Request) {
		erawr := r.ParseForm()
		if erawr != nil {
			fmt.Println("submit enableemail failed")
			os.Exit(1)
		}
		value := r.FormValue("value")
		if value == "true" {
			EnableEmail = true
			utils.WriteEnableEmail("true")
		} else {
			EnableEmail = false
			utils.WriteEnableEmail("false")
		}
	})

	http.HandleFunc("/customsoundcard", func(w http.ResponseWriter, r *http.Request) {
		erawr := r.ParseForm()
		if erawr != nil {
			fmt.Println("submit  customsoundcard failed")
			os.Exit(1)
		}
		value := r.FormValue("value")
		if value == "true" {
			EnableEmail = true
			utils.WriteCustomSoundCard("true")
		} else {
			EnableEmail = false
			utils.WriteCustomSoundCard("false")
		}
	})

	http.HandleFunc("/newemail", func(w http.ResponseWriter, r *http.Request) {
		erawr := r.ParseForm()
		if erawr != nil {
			fmt.Println("submit newemail failed")
			os.Exit(1)
		}
		value := r.FormValue("value")
		utils.WriteEmail(value)
	})

	http.HandleFunc("/submitcolors", func(w http.ResponseWriter, r *http.Request) {
		erawr := r.ParseForm()
		if erawr != nil {
			fmt.Println("submitcolors failed")
			os.Exit(1)
		}
		value := r.FormValue("value")
		fmt.Println(value)
		utils.ColorUpdate(value)
		hex, err := colors.ParseHEX(value)
		if err != nil {
			fmt.Println("Error Parsing hex color")
		}
		fmt.Println(hex.ToRGB())
	})

	http.HandleFunc("/submitenableled", func(w http.ResponseWriter, r *http.Request) {
		erawr := r.ParseForm()
		if erawr != nil {
			fmt.Println("submitenabled failed")
			os.Exit(1)
		}
		value := r.FormValue("value")
		if value == "true" {
			EnableEmail = true
			utils.WriteEnableLed("true")
		} else {
			EnableEmail = false
			utils.WriteEnableLed("false")
		}
	})

	//Pass on the AJAX post /upload handler to the uploadHandler() function
	http.HandleFunc("/upload", uploadHandler)
	log.Println("Listening...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
