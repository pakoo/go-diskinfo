package  diskinfo

import (
"io"
"bytes"
"fmt"
"os/exec"
"time"
"bufio"
"regexp"
"strings"
"github.com/shirou/gopsutil/disk"
"strconv"
"net"
"io/ioutil"
)

type cmdRunner struct{}
type disks map[string]map[string]string
type network_card map[string]map[string]string

func New() *cmdRunner {
	return &cmdRunner{}
}

func (c *cmdRunner) Run(cmd string, args []string) (io.Reader, error) {
	command := exec.Command(cmd, args...)
	resCh := make(chan []byte)
	errCh := make(chan error)
	go func() {
		out, err := command.CombinedOutput()
		if err != nil {
			errCh <- err
		}
		resCh <- out
	}()
	timer := time.After(2 * time.Second)
	select {
	case err := <-errCh:
		return nil, err
	case res := <-resCh:
		return bytes.NewReader(res), nil
	case <-timer:
		return nil, fmt.Errorf("time out (cmd:%v args:%v)", cmd, args)
	}
}

func (c *cmdRunner) Exec(cmd string, args []string) string {
	command := exec.Command(cmd, args...)
	outputBytes, _ := command.CombinedOutput()
	//if err != nil {
	//	log.Error(err)
	//}
	return string(outputBytes[:])
}


func parser_iostat(r io.Reader) disks{
	iostat_info := make(disks)
	scan :=bufio.NewScanner(r)
	for  scan.Scan() {
		fields_list := []string{"rrqm/s","wrqm/s","r/s","w/s","rkB/s","wkB/s","avgrq-sz","avgqu-sz","await","r_await","w_await","svctm","%util"}
		line := scan.Text()
		fields := strings.Fields(line)
		if strings.HasPrefix(line,"sd") {
			iostat_info[fields[0]] = make(map[string]string)
			for i,f :=range fields_list {
				iostat_info[fields[0]][f] = fields[i+1]
			}
		}
	}
	//marshal, _ := json.Marshal(iostat_info)
	//fmt.Println(string(marshal))
	return  iostat_info
}

func stringInSlice(str string, list []string) bool {
	for _, v := range list {
		if v == str {
			return true
		}
	}
	return false
}

func parser_lsblk(r io.Reader)map[string]map[string]string{
	/*  only for Linux
		cmd: lsblk -P -o NAME,KNAME,MODEL,UUID,SIZE,ROTA,FSTYPE,TYPE,MOUNTPOINT,MAJ:MIN
	*/
	var lsblk = make(disks)
	re := regexp.MustCompile("([A-Z]+)=(?:\"(.*?)\")")
	scan :=bufio.NewScanner(r)
	for  scan.Scan(){
		//pre := []string{"sd","hd"}
		var disk_name = ""
		disk := make(map[string]string)
		raw := scan.Text()
		sr := re.FindAllStringSubmatch(raw,-1)
		for i,k :=range sr{
			k[1] = strings.ToLower(k[1])
			k[2] = strings.ToLower(k[2])
			if i == 0{
				disk_name = k[2]
			}
			disk[k[1]] = k[2]
			if (k[1]=="mountpoint" && strings.HasPrefix(k[2],"/"))  {
				usage := DiskUsage(k[2])
				disk["used"] = string(usage)
			}
		}
		if disk["type"] == "disk" {
			disk_path := fmt.Sprintf("/sys/block/%s/queue/rotational", disk_name)
			buf, err := ioutil.ReadFile(disk_path)
			if err != nil {
				fmt.Println(err)
			} else {
				disk["disk_rotational"]=strings.TrimSpace(string(buf))
			}
		}
		lsblk[disk_name] = disk
	}
	//j,_:= json.MarshalIndent(lsblk,""," ")
	//pj := string(j)
	//fmt.Println(pj)
	return  lsblk

}

func Lsblk()disks{
	var cmdrun = cmdRunner{}
	rr,err := cmdrun.Run("lsblk",[]string{"-P","-b","-o","NAME,KNAME,MODEL,PARTUUID,SIZE,ROTA,TYPE,MOUNTPOINT,MAJ:MIN,PKNAME"})
	if err != nil {
		fmt.Println(err)
	}
	disks := parser_lsblk(rr)
	return  disks
}

func Iostat()disks{
	var cmdrun = cmdRunner{}
	rr,err := cmdrun.Run("iostat",[]string{"-x"})
	if err != nil {
		fmt.Println(err)
	}
	disks := parser_iostat(rr)
	return  disks
}

func DiskUsage(path string) string{
	usage,_ := disk.Usage(path)
	return  strconv.Itoa(int(usage.Used))
}

func Lsnet()network_card{
	nc := make(network_card)
	cmdrun := cmdRunner{}

	a, err := net.Interfaces()
	if err != nil {
		fmt.Println(err)
	}
	var buf = new(bytes.Buffer)
	for i := 0; i < len(a); i++ {
		nc[a[i].Name] = make(map[string]string)
		nc[a[i].Name]["name"] = a[i].Name
		nc[a[i].Name]["flags"] = a[i].Flags.String()
		flags := a[i].Flags.String()
		if strings.Contains(flags, "up") && strings.Contains(flags, "broadcast") {
			nc[a[i].Name]["up"] = "1"
			nc[a[i].Name]["hardware_addr"] = a[i].HardwareAddr.String()
			addrs,_ := a[i].Addrs()
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
					if ip.To4() != nil{
						nc[a[i].Name]["ip"] = ip.String()
					}
				case *net.IPAddr:
					ip = v.IP
					if ip.To4() != nil{
						nc[a[i].Name]["ip"] = ip.String()
					}
				}
			}
		} else {
			nc[a[i].Name]["up"] = "0"
		}
		rr,err := cmdrun.Run("ethtool",[]string{a[i].Name})
		if err != nil {
			fmt.Println(err)
		}
		buf.ReadFrom(rr)
		if strings.Contains(buf.String(),"Link detected: yes") {
			nc[a[i].Name]["linked"] = "1"
		}else {
			nc[a[i].Name]["linked"] = "0"
		}
		buf.Reset()

	}
	//marshal, _ := json.Marshal(nc)
	//fmt.Println(string(marshal))
	return  nc
}



/*
func main(){
	var rr io.Reader
	var cmdrun = cmdRunner{}
	//rr,err := cmdrun.Run("iostat",[]string{"-x","-k"})
	rr,err := cmdrun.Run("lsblk",[]string{"-P","-b","-o","NAME,KNAME,MODEL,UUID,SIZE,ROTA,TYPE,MOUNTPOINT,MAJ:MIN"})
	if err != nil {
		fmt.Println(err)
	}
	//var buf = new(bytes.Buffer)
	//buf.ReadFrom(rr)
	//fmt.Println(buf.String())

	//f,_:=os.Open("./data/lsblk")
	//defer f.Close()

	var disks map[string]map[string]string
	disks = parser_lsblk(rr)
	fmt.Println(disks)
}
*/
