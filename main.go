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

// localRecords adalah "database" sederhana untuk domain-domain yang ingin direspon secara lokal.
// Key adalah nama domain (lengkap dengan titik di akhir), value adalah alamat IP.
// HANYA domain yang ada di map ini yang akan direspon secara lokal.
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
	for _, q := range r.Question {
		queryNameLower := strings.ToLower(q.Name)
		log.Printf("Query untuk: %s (Tipe: %s)", q.Name, dns.TypeToString[q.Qtype])

		// LANGKAH 1: Cek langsung di localRecords.
		ip, found := localRecords[queryNameLower]

		if found {
			// LANGKAH 2a: Ditemukan di lokal, jawab dengan IP lokal dan STOP.
			log.Printf("  -> Ditemukan di localRecords: %s -> %s", q.Name, ip)
			rr, err := dns.NewRR(fmt.Sprintf("%s A %s", q.Name, ip))
			if err == nil {
				m.Answer = append(m.Answer, rr)
			}
		} else {
			// LANGKAH 2b: Tidak ditemukan di lokal, forward ke upstream.
			log.Printf("  -> Tidak ditemukan di localRecords. Meneruskan ke upstream...")

			c := new(dns.Client)
			c.Net = "udp"
			c.Timeout = 2 * time.Second

			var forwardedResponse *dns.Msg
			var err error
			for _, forwarder := range dnsForwarders {
				// Gunakan r (pesan asli) untuk forwarding
				forwardedResponse, _, err = c.Exchange(r, forwarder)
				if err == nil {
					log.Printf("  -> Berhasil mendapat jawaban dari %s", forwarder)
					break // Keluar dari loop jika berhasil
				}
				log.Printf("  -> Gagal menghubungi %s: %v", forwarder, err)
			}

			if err != nil {
				log.Printf("  -> Semua upstream gagal. Tidak dapat menjawab query untuk %s", q.Name)
				// Jika semua forwarder gagal, kita bisa mengembalikan pesan ServerFailure
				m.SetRcode(r, dns.RcodeServerFailure)
			} else {
				// Jika berhasil, gabungkan jawaban dari upstream ke pesan balasan kita.
				m.Answer = append(m.Answer, forwardedResponse.Answer...)
			}
		}
	}

	// Kirimkan balaban ke klien.
	w.WriteMsg(m)
}

func main() {
	// Siapkan server DNS
	server := &dns.Server{
		Addr:    listenAddr,
		Net:     "udp",
		Handler: dns.HandlerFunc(handleDNSRequest),
	}

	log.Printf("Memulai DNS Forwarder pada %s", listenAddr)
	log.Printf("Records lokal yang dikonfigurasi (hanya ini yang akan direspon secara lokal):")
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
