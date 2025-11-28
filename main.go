package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// --- KONFIGURASI ---
// Variabel-variabel ini bisa Anda ubah sesuai kebutuhan.

// listenAddr adalah alamat dan port tempat server DNS ini akan berjalan.
// Port 53 adalah port standar DNS dan membutuhkan akses root untuk dijalankan.
const listenAddr = ":53"

// dnsForwarders adalah daftar server DNS publik yang akan digunakan untuk meneruskan permintaan.
var dnsForwarders = []string{
	"8.8.8.8:53", // Google DNS
	"1.1.1.1:53", // Cloudflare DNS
}

// localRecords adalah "database" sederhana untuk zona lokal kita.
// Key adalah nama domain (lengkap dengan titik di akhir), value adalah alamat IP.
var localRecords = map[string]string{
	"pacebook.com.":  "10.180.53.85",
	"klikbeca.com.":  "10.180.53.227",
	"login.hotspot.": "10.180.52.198",
}

// --- AKHIR KONFIGURASI ---

// handleDNSRequest adalah fungsi utama yang menangani setiap permintaan DNS.
func handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = false

	// Iterasi melalui setiap pertanyaan dalam pesan request.
	// Satu pesan DNS bisa berisi beberapa pertanyaan.
	for _, q := range r.Question {
		log.Printf("Query untuk: %s (Tipe: %s)", q.Name, dns.TypeToString[q.Qtype])

		// Cek apakah ini adalah permintaan untuk zona lokal kita.
		// Kita gunakan strings.HasSuffix untuk menangkap semua subdomain.
		if strings.HasSuffix(strings.ToLower(q.Name), ".lan.") {
			// Ini adalah zona lokal, cari di localRecords.
			ip, found := localRecords[strings.ToLower(q.Name)]
			if found {
				log.Printf("  -> Ditemukan di zona lokal: %s -> %s", q.Name, ip)
				// Buat sebuah A record (Address Record) untuk jawabannya.
				rr, err := dns.NewRR(fmt.Sprintf("%s A %s", q.Name, ip))
				if err == nil {
					m.Answer = append(m.Answer, rr)
				}
			} else {
				log.Printf("  -> Tidak ditemukan di zona lokal: %s", q.Name)
			}
		} else {
			// Ini bukan zona lokal, teruskan (forward) ke server DNS upstream.
			log.Printf("  -> Meneruskan ke upstream...")

			// Buat koneksi baru ke server upstream untuk setiap pertanyaan.
			// Untuk performa lebih baik, Anda bisa menggunakan connection pooling.
			c := new(dns.Client)
			c.Net = "udp" // Gunakan UDP untuk query standar
			c.Timeout = 2 * time.Second

			// Coba setiap forwarder sampai ada yang berhasil.
			var forwardedResponse *dns.Msg
			var err error
			for _, forwarder := range dnsForwarders {
				forwardedResponse, _, err = c.Exchange(r, forwarder)
				if err == nil {
					log.Printf("  -> Berhasil mendapat jawaban dari %s", forwarder)
					break // Keluar dari loop jika berhasil
				}
				log.Printf("  -> Gagal menghubungi %s: %v", forwarder, err)
			}

			if err != nil {
				// Jika semua forwarder gagal, kita tidak bisa menjawab.
				log.Printf("  -> Semua upstream gagal. Tidak dapat menjawab query untuk %s", q.Name)
				// Kita bisa mengembalikan pesan kosong (yang akan dianggap NXDOMAIN oleh klien)
				// atau mengatur Rcode ke ServerFailure.
				m.Rcode = dns.RcodeServerFailure
			} else {
				// Jika berhasil, gabungkan jawaban dari upstream ke pesan balasan kita.
				m.Answer = append(m.Answer, forwardedResponse.Answer...)
			}
		}
	}

	// Kirimkan balasan ke klien.
	w.WriteMsg(m)
}

func main() {
	// Siapkan server DNS
	server := &dns.Server{
		Addr:    listenAddr,
		Net:     "udp", // DNS biasanya berjalan di atas UDP
		Handler: dns.HandlerFunc(handleDNSRequest),
	}

	log.Printf("Memulai DNS Forwarder pada %s", listenAddr)
	log.Printf("Zona lokal yang dikonfigurasi:")
	for domain := range localRecords {
		log.Printf("  - %s -> %s", domain, localRecords[domain])
	}
	log.Printf("DNS Upstream: %v", dnsForwarders)

	// Jalankan server
	err := server.ListenAndServe()
	if err != nil {
		log.Fatalf("Gagal menjalankan server: %v\n", err)
	}
}
