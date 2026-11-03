package ui

import (
	"net"
	"net/netip"
	"sort"
	"strings"

	"github.com/qwkejkqwje1/dota-voicebox/internal/qr"
)

// LANAddr — адрес компьютера в локальной сети.
type LANAddr struct {
	IP      string
	Iface   string
	Primary bool // через эту сеть идёт интернет — скорее всего, тот самый Wi-Fi или кабель от роутера
	VPN     bool // похоже на VPN — с телефона по такому адресу, скорее всего, не открыть
}

// виртуальные адаптеры (виртуальные машины, WSL, Docker, Bluetooth) — телефон через них не достучится
var virtualIfaces = []string{"vethernet", "virtualbox", "vbox", "vmware", "hyper-v", "wsl", "docker", "bluetooth", "loopback", "npcap"}

// VPN-адаптеры: телефон в домашней сети через них тоже не достучится, а интернет при включённом VPN
// идёт как раз через них — поэтому «основным» такой адрес не считаем и показываем последним
var (
	vpnIfaces   = []string{"vpn", "wireguard", "zerotier", "nordlynx", "wintun", "tailscale", "hamachi", "radmin", "amnezia", "outline", "warp", "tap-", "tunnel"}
	vpnPrefixes = []string{"tun", "tap", "wg", "ppp", "utun", "zt"}
)

// ifaceKind — виртуальный ли это адаптер или VPN (по имени, другого способа без лишних прав нет).
func ifaceKind(name string) (virtual, vpn bool) {
	name = strings.ToLower(name)
	for _, v := range virtualIfaces {
		if strings.Contains(name, v) {
			virtual = true
		}
	}
	for _, v := range vpnIfaces {
		if strings.Contains(name, v) {
			vpn = true
		}
	}
	for _, p := range vpnPrefixes {
		if strings.HasPrefix(name, p) {
			vpn = true
		}
	}
	return virtual, vpn
}

// LANAddrs — частные IPv4-адреса компьютера: основной первым.
func LANAddrs() []LANAddr {
	primary := ""
	// UDP «соединение» не шлёт пакетов — система лишь выбирает маршрут и наш адрес в нём
	if c, err := net.Dial("udp4", "192.0.2.1:9"); err == nil {
		if a, ok := c.LocalAddr().(*net.UDPAddr); ok {
			primary = a.IP.String()
		}
		c.Close()
	}
	var out []LANAddr
	seen := map[string]bool{}
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		virtual, isVPN := ifaceKind(ifc.Name)
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			pn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip, ok := netip.AddrFromSlice(pn.IP)
			if !ok {
				continue
			}
			ip = ip.Unmap()
			if !ip.Is4() || !ip.IsPrivate() || seen[ip.String()] {
				continue
			}
			s := ip.String()
			if virtual && s != primary {
				continue
			}
			seen[s] = true
			out = append(out, LANAddr{IP: s, Iface: ifc.Name, Primary: s == primary && !isVPN, VPN: isVPN})
		}
	}
	return sortLAN(out)
}

// sortLAN — основной адрес первым, VPN — в конце, остальные в порядке системы.
func sortLAN(out []LANAddr) []LANAddr {
	rank := func(a LANAddr) int {
		switch {
		case a.Primary:
			return 0
		case a.VPN:
			return 2
		}
		return 1
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	return out
}

// qrSVG — QR-код адреса (SVG); "" — не получилось.
func qrSVG(text string) string {
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return ""
	}
	return c.SVG("#000", "#fff")
}
