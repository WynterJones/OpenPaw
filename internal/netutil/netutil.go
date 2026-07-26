package netutil

import "net"

// GetLANIP returns the first non-loopback, non-Tailscale IPv4 address.
func GetLANIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			if ip[0] == 100 && ip[1] >= 64 && ip[1] <= 127 {
				continue
			}
			return ip.String()
		}
	}
	return ""
}

// tailscaleCGNAT is the range Tailscale assigns tailnet addresses from.
var tailscaleCGNAT = &net.IPNet{
	IP:   net.IPv4(100, 64, 0, 0),
	Mask: net.CIDRMask(10, 32),
}

// GetTailscaleIP returns the first Tailscale CGNAT (100.64.0.0/10) IPv4 address.
func GetTailscaleIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			// Must index the 4-byte form. An IPv4 address from Addrs() is
			// often the 16-byte representation, where ip[0] is a leading zero
			// rather than the first octet — testing ip[0] directly silently
			// never matched, which left tailnet detection dead on macOS.
			ip4 := ip.To4()
			if ip4 == nil {
				continue
			}
			if tailscaleCGNAT.Contains(ip4) {
				return ip4.String()
			}
		}
	}
	return ""
}
