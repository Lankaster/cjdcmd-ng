package main

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/inhies/go-cjdns/admin"
	"github.com/inhies/go-cjdns/config"
	"io/ioutil"
	"math/rand"
	"net"
	"os/user"
	"regexp"
	"strconv"
	"strings"
)

const (
	ipRegex   = "^fc[a-f0-9]{1,2}:([a-f0-9]{0,4}:){2,6}[a-f0-9]{1,4}$"
	pathRegex = "([0-9a-f]{4}\\.){3}[0-9a-f]{4}"
	hostRegex = "^([a-zA-Z0-9]([a-zA-Z0-9\\-\\.]{0,}[a-zA-Z0-9]))$"
)

type CjdnsAdmin struct {
	Address  string `json:"addr"`
	Port     int    `json:"port"`
	Password string `json:"password"`
	Config   string `json:"config,omitempty"`
}

// gotYes will read from stdin and if it is any variation of 'y' or 'yes' then it returns true
// If defaultYes is set to true and the user presses enter without entering anything else it returns true
func gotYes(defaultYes bool) bool {
	var choice string
	n, _ := fmt.Scanln(&choice)
	if n == 0 {
		if defaultYes {
			return true
		} else {
			return false
		}
	}
	if strings.ToLower(choice) == "y" || strings.ToLower(choice) == "yes" {
		return true
	}
	return false
}

// Reads the .cjdnsadmin file and returns the structured contents
func readCjdnsadmin(file string) (admin *CjdnsAdmin, err error) {
	rawFile, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	raw, err := stripComments(rawFile)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(raw, &admin)
	if err != nil {
		return nil, err
	}
	if err != nil {
		// BUG(inhies): Find a better way of dealing with these errors.
		if e, ok := err.(*json.SyntaxError); ok {
			// BUG(inhies): Instead of printing x amount of characters, print the previous and following 2 lines
			fmt.Println("Invalid JSON") //" at byte", e.Offset, "(after stripping comments...)")
			fmt.Println("----------------------------------------")
			fmt.Println(string(raw[e.Offset-60 : e.Offset+60]))
			fmt.Println("----------------------------------------")
		} else if _, ok := err.(*json.InvalidUTF8Error); ok {
			fmt.Println("Invalid UTF-8")
		} else if e, ok := err.(*json.InvalidUnmarshalError); ok {
			fmt.Println("Invalid unmarshall type", e.Type)
			fmt.Println(err)
		} else if e, ok := err.(*json.UnmarshalFieldError); ok {
			fmt.Println("Invalid unmarshall field", e.Field, e.Key, e.Type)
		} else if e, ok := err.(*json.UnmarshalTypeError); ok {
			fmt.Println("Invalid JSON")
			fmt.Println("Expected", e.Type, "but received a", e.Value)
			fmt.Println("I apologize for not being more helpful")
		} else if e, ok := err.(*json.UnsupportedTypeError); ok {
			fmt.Println("Invalid JSON")
			fmt.Println("I am unable to utilize type", e.Type)
		} else if e, ok := err.(*json.UnsupportedValueError); ok {
			fmt.Println("Invalid JSON")
			fmt.Println("I am unable to utilize value", e.Value, e.Str)
		}
		return nil, err
	}
	return

}

// Reads the configuration file specified in global variable File
// and sets the admin credentials
func readConfig() (conf *config.Config, err error) {
	conf, err = config.LoadMinConfig(File)
	if err != nil || len(conf.Admin.Password) == 0 {
		return
	}

	AdminPassword = conf.Admin.Password
	AdminBind = conf.Admin.Bind

	return
}

// Attempt to connect to cjdns 
func adminConnect() (user *admin.Admin, err error) {
	// If nothing else has already set this
	if AdminBind == "" || AdminPassword == "" {
		// If we still have no idea which configuration file to use
		if File == "" {
			// Try getting it from the .cjdnsadmin file
			cjdAdmin, err := loadCjdnsadmin()
			if err != nil {
				// .cjdnsadmin failed us, 
				//_, err = readConfig()
				//if err != nil {
				err = fmt.Errorf("Unable to load configuration file:", err.Error())
				return nil, err
				//}
			}
			//File = cjdAdmin.Config
			//if File == "" {
			//	err = fmt.Errorf("Please specify the configuration file in your .cjdnsadmin file or pass the --file flag.")
			//	return nil, err
			//}

			// Set the admin credentials from the .cjdnsadmin file
			AdminPassword = cjdAdmin.Password
			AdminBind = cjdAdmin.Address + ":" + strconv.Itoa(cjdAdmin.Port)

			// If File and OutFile aren't already set, set them
			// Note that they could still be empty if the "config" 
			// entry isnt in .cjdnsadmin
			if File == "" {
				File = cjdAdmin.Config
			}

			if OutFile == "" {
				OutFile = cjdAdmin.Config
			}
		} else {
			// File is set so use it
			_, err = readConfig()
			if err != nil {
				err = fmt.Errorf("Unable to load configuration file:", err.Error())
				return nil, err
			}
		}
	}
	user, err = admin.Connect(AdminBind, AdminPassword)
	if err != nil {
		if e, ok := err.(net.Error); ok {
			if e.Timeout() {
				fmt.Println("\nConnection timed out")
			} else if e.Temporary() {
				fmt.Println("\nTemporary error (not sure what that means!)")
			} else {
				fmt.Println("\nUnable to connect to cjdns:", e)
			}
		} else {
			fmt.Println("\nError:", err)
		}
		return
	}
	return
}

// Connect to a 
//func connect() (user *admin.Admin, err error) {
//	user, err = adminConnect()
//	if err != nil {
//		return
//	}
//	return
//}

// Attempt to read the .cjdnsadmin file from the users home directory
func loadCjdnsadmin() (cjdnsAdmin *CjdnsAdmin, err error) {
	tUser, err := user.Current()
	if err != nil {
		return
	}
	cjdnsAdmin, err = readCjdnsadmin(tUser.HomeDir + "/.cjdnsadmin")
	if err != nil {
		return
	}
	return
}

// Fills out an IPv6 address to the full 32 bytes
// This shouldn't be needed in newer versions of cjdns

func padIPv6(ip net.IP) string {
	raw := hex.EncodeToString(ip)
	parts := make([]string, len(raw)/4)
	for i := range parts {
		parts[i] = raw[i*4 : (i+1)*4]
	}
	return strings.Join(parts, ":")
}

// Dumps the entire routing table and structures it
func getTable(user *admin.Admin) (table []*Route) {
	page := 0
	var more int64
	table = make([]*Route, 0)
	for more = 1; more != 0; page++ {
		response, err := admin.NodeStore_dumpTable(user, page)
		if err != nil {
			fmt.Printf("%v\n", err)
			return
		}
		// If an error field exists, and we have an error, return it
		if _, ok := response["error"]; ok {
			if response["error"] != "none" {
				err = fmt.Errorf(response["error"].(string))
				fmt.Printf("Error: %v\n", err)
				return
			}
		}
		//Thanks again to SashaCrofter for the table parsing
		rawTable := response["routingTable"].([]interface{})
		for i := range rawTable {
			item := rawTable[i].(map[string]interface{})
			rPath := item["path"].(string)
			sPath := strings.Replace(rPath, ".", "", -1)
			bPath, err := hex.DecodeString(sPath)
			if err != nil || len(bPath) != 8 {
				//If we get an error, or the
				//path is not 64 bits, discard.
				//This should also prevent
				//runtime errors.
				continue
			}
			path := binary.BigEndian.Uint64(bPath)
			table = append(table, &Route{
				IP:      item["ip"].(string),
				RawPath: path,
				Path:    rPath,
				RawLink: item["link"].(int64),
				Link:    float64(item["link"].(int64)) / magicalLinkConstant,
				Version: item["version"].(int64),
			})

		}

		if response["more"] != nil {
			more = response["more"].(int64)
		} else {
			break
		}
	}

	return
}

type Target struct {
	Target   string
	Supplied string
}

func validIP(input string) (result bool) {
	result, _ = regexp.MatchString(ipRegex, input)
	return
}
func validPath(input string) (result bool) {
	result, _ = regexp.MatchString(pathRegex, input)
	return
}
func validHost(input string) (result bool) {
	result, _ = regexp.MatchString(hostRegex, input)
	return
}

// Sets target.Target to the requried IP or cjdns path
func setTarget(data []string, usePath bool) (target Target, err error) {
	if len(data) == 0 {
		err = fmt.Errorf("Invalid target specified")
		return
	}
	input := data[0]
	if input != "" {

		if validIP(input) {
			target.Supplied = data[0]
			target.Target = padIPv6(net.ParseIP(input))
			return

		} else if validPath(input) && usePath {
			target.Target = input
			target.Supplied = data[0]
			return

		} else if validHost(input) {
			var ips []string
			ips, err = resolveHost(input)
			if err != nil {
				return
			}
			// Return the first result
			for _, addr := range ips {
				target.Target = addr
				target.Supplied = input
				return
			}

		} else {
			err = fmt.Errorf("Invalid IPv6 address, cjdns path, or hostname")
			return
		}
	}

	if usePath {
		err = fmt.Errorf("You must specify an IPv6 address, hostname or cjdns path")
		return
	}
	err = fmt.Errorf("You must specify an IPv6 address or hostname")
	return
}

func usage() {
	println("cjdcmd version ", Version)
	println("")
	println("Usage: cjdcmd command [arguments]")
	println("")
	println("The commands are:")
	println("")
	println("ping <ipv6 address, hostname, or routing path>       sends a cjdns ping to the specified node")
	println("route <ipv6 address, hostname, or routing path>      prints out all routes to an IP or the IP to a route")
	println("traceroute <ipv6 address, hostname, or routing path> [-t timeout] performs a traceroute by pinging each known hop to the target on all known paths")
	println("ip <cjdns public key>                                converts a cjdns public key to the corresponding IPv6 address")
	println("host <ipv6 address or hostname>                      returns a list of all know IP address for the specified hostname or the hostname for an address")
	println("cjdnsadmin <-file>                                   creates a .cjdnsadmin file in your home directory using the specified cjdroute.conf as input")
	println("addpeer [-file] [-outfile] '<json peer details>'     adds the peer details to your config file")
	println("addpass [-file] [-outfile] [password]                adds the password to the config if one was supplied, or generates one and then adds")
	println("cleanconfig [-file] [-outfile]                       strips all comments from the config file and then saves it nicely formatted")
	println("log [-l level] [-logfile file] [-line line]          prints cjdns log to stdout")
	println("passgen                                              generates a random alphanumeric password between 15 and 50 characters in length")
	println("peers                                                displays a list of currently connected peers")
	println("dump                                                 dumps the routing table to stdout")
	println("kill                                                 tells cjdns to gracefully exit")
	println("memory                                               returns the number of bytes of memory the router has allocated")
	println("")
	println("Use `cjdcmd --help` for a list of flags.")
}

// Returns a random alphanumeric string where length is <= max >= min
func randString(min, max int) string {
	r := myRand(min, max, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
	return r
}

// Returns a random character from the specified string where length is <= max >= min
func myRand(min, max int, char string) string {

	var length int

	if min < max {
		length = min + rand.Intn(max-min)
	} else {
		length = min
	}

	buf := make([]byte, length)
	for i := 0; i < length; i++ {
		buf[i] = char[rand.Intn(len(char)-1)]
	}
	return string(buf)
}

func stripComments(b []byte) ([]byte, error) {
	regComment, err := regexp.Compile("(?s)//.*?\n|/\\*.*?\\*/")
	if err != nil {
		return nil, err
	}
	out := regComment.ReplaceAllLiteral(b, nil)
	return out, nil
}
