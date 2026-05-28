# Dokumentasi API Modul SAFE Backend (Aplikasi Emergency & Security)

Berikut adalah dokumen spesifikasi teknis dan integrasi API untuk Modul Backend **SAFE**. Seluruh endpoint di bawah ini dikembangkan menggunakan **Go (Golang)** dengan framework **Gin** dan database **PostgreSQL**.

---

| No | API | Informasi |
| :--- | :--- | :--- |
| **1** | **Nama:** | Register Akun Baru |
| | **URL** | `/api/register` |
| | **Method** | `POST` |
| | **Type** | `JSON` |
| | **Authentifikasi** | `-` |
| | **Parameters** | `name` (string, required)<br>`email` (string, required)<br>`password` (string, required, min 6)<br>`phone_number` (string, required)<br>`fcm_token` (string, optional) |
| | **Return value** | **JSON**<br><br>**Success (201 Created):**<br>- `token` (string)<br>- `user` (object: `user_id`, `name`, `email`, `phone_number`, `profile_image`, `fcm_token`, `created_at`)<br><br>**Gagal (400 Bad Request / 409 Conflict):**<br>- `error` (string: pesan kegagalan/email sudah terdaftar) |
| | **Keterangan** | Digunakan untuk mendaftarkan akun baru pengguna ke dalam sistem. |
| :--- | :--- | :--- |
| **2** | **Nama:** | Login / Otentikasi |
| | **URL** | `/api/login` |
| | **Method** | `POST` |
| | **Type** | `JSON` |
| | **Authentifikasi** | `-` |
| | **Parameters** | `email` (string, required)<br>`password` (string, required) |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `token` (string JWT)<br>- `user` (object: `user_id`, `name`, `email`, `phone_number`, `profile_image`, `fcm_token`, `created_at`)<br><br>**Gagal (400 Bad Request / 401 Unauthorized):**<br>- `error` (string: sandi/email salah atau format tidak valid) |
| | **Keterangan** | Digunakan untuk masuk (*login*) ke dalam sistem dan mendapatkan Token Akses JWT. |
| :--- | :--- | :--- |
| **3** | **Nama:** | Login / Otentikasi via Google |
| | **URL** | `/api/auth/google` |
| | **Method** | `POST` |
| | **Type** | `JSON` |
| | **Authentifikasi** | `-` |
| | **Parameters** | `id_token` (string, optional)<br>`email` (string, optional - simulasi)<br>`name` (string, optional - simulasi) |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `token` (string JWT)<br>- `user` (object: `user_id`, `name`, `email`, `phone_number`, `profile_image`, `fcm_token`, `created_at`)<br><br>**Gagal (400 Bad Request / 401 Unauthorized / 500 Server Error):**<br>- `error` (string: pesan kegagalan) |
| | **Keterangan** | Otentikasi menggunakan Google OAuth Token. Mendukung mode simulasi (fallback) untuk testing lokal. |
| :--- | :--- | :--- |
| **4** | **Nama:** | Update Profile Dasar |
| | **URL** | `/api/profile` |
| | **Method** | `PUT` |
| | **Type** | `JSON` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `name` (string, required)<br>`phone_number` (string, required)<br>`profile_image` (string base64, optional) |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `message` (string: "Profil berhasil diperbarui")<br>- `user` (object: data pengguna terbaru)<br><br>**Gagal (400 Bad Request / 401 Unauthorized / 500 Server Error):**<br>- `error` (string: pesan kegagalan) |
| | **Keterangan** | Digunakan untuk mengubah nama, nomor handphone, dan mengunggah foto profil kustom pengguna. |
| :--- | :--- | :--- |
| **5** | **Nama:** | Update Token FCM (Push Notification) |
| | **URL** | `/api/profile/fcm` |
| | **Method** | `PUT` |
| | **Type** | `JSON` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `fcm_token` (string, required) |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `message` (string: "FCM Token berhasil diperbarui")<br><br>**Gagal (400 Bad Request / 401 Unauthorized / 500 Server Error):**<br>- `error` (string: pesan kegagalan) |
| | **Keterangan** | Memperbarui token Firebase Cloud Messaging milik pengguna agar dapat menerima pemberitahuan/notifikasi darurat. |
| :--- | :--- | :--- |
| **6** | **Nama:** | Update Lokasi Terkini |
| | **URL** | `/api/location` |
| | **Method** | `PUT` |
| | **Type** | `JSON` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `latitude` (float64, required)<br>`longitude` (float64, required) |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `message` (string: "Lokasi berhasil diperbarui")<br><br>**Gagal (400 Bad Request / 401 Unauthorized / 500 Server Error):**<br>- `error` (string: pesan kegagalan) |
| | **Keterangan** | Mengirimkan koordinat GPS terbaru milik pengguna ke sistem untuk pelacakan lokasi regular. |
| :--- | :--- | :--- |
| **7** | **Nama:** | Get Medical Profile (Data Medis) |
| | **URL** | `/api/profile/medical` |
| | **Method** | `GET` |
| | **Type** | `-` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `-` |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `medical_id` (string)<br>- `user_id` (string)<br>- `blood_type` (string)<br>- `medical_notes` (string)<br><br>**Gagal (401 Unauthorized / 500 Server Error):**<br>- `error` (string: pesan kegagalan) |
| | **Keterangan** | Mengambil data riwayat penyakit, alergi, dan golongan darah milik pengguna saat ini. |
| :--- | :--- | :--- |
| **8** | **Nama:** | Upsert Medical Profile |
| | **URL** | `/api/profile/medical` |
| | **Method** | `POST` |
| | **Type** | `JSON` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `blood_type` (string, required)<br>`medical_notes` (string, required) |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `medical_id` (string)<br>- `user_id` (string)<br>- `blood_type` (string)<br>- `medical_notes` (string)<br><br>**Gagal (400 Bad Request / 401 Unauthorized / 500 Server Error):**<br>- `error` (string: pesan kegagalan) |
| | **Keterangan** | Menyimpan atau memperbarui data riwayat penyakit/alergi dan golongan darah pengguna. |
| :--- | :--- | :--- |
| **9** | **Nama:** | Pencarian Pengguna Kontak |
| | **URL** | `/api/users/search` |
| | **Method** | `GET` |
| | **Type** | `Query Parameter` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `q` (string query, required: email atau nomor telepon teman) |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `users` (array object: `id`, `name`, `phone_number`, `profile_image`, `status` [status hubungan: 'Tersambung', 'Menunggu Konfirmasi', atau ''])<br><br>**Gagal (400 Bad Request / 401 Unauthorized / 500 Server Error):**<br>- `error` (string: pesan kegagalan) |
| | **Keterangan** | Mencari pengguna lain di sistem berdasarkan No. HP/Email untuk ditambahkan sebagai Kontak Darurat. |
| :--- | :--- | :--- |
| **10** | **Nama:** | Tambah Kontak Darurat |
| | **URL** | `/api/contacts` |
| | **Method** | `POST` |
| | **Type** | `JSON` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `target_user_id` (string, required) |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `message` (string: "Permintaan kontak darurat berhasil dikirim")<br><br>**Gagal (400 Bad Request / 401 Unauthorized):**<br>- `error` (string: tidak bisa menambahkan diri sendiri atau sudah terhubung) |
| | **Keterangan** | Mengirimkan permintaan tautan/persetujuan kontak darurat ke pengguna lain. |
| :--- | :--- | :--- |
| **11** | **Nama:** | Ambil Daftar Kontak Aktif |
| | **URL** | `/api/contacts` |
| | **Method** | `GET` |
| | **Type** | `-` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `-` |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `contacts` (array object: `id`, `name`, `phone_number`, `profile_image`, `status` [selalu 'Tersambung'], `last_latitude`, `last_longitude`, `last_location_update`)<br><br>**Gagal (401 Unauthorized / 500 Server Error):**<br>- `error` (string: pesan kegagalan) |
| | **Keterangan** | Menampilkan daftar semua teman yang sudah terhubung resmi sebagai kontak darurat timbal-balik beserta koordinat lokasi terakhir mereka. |
| :--- | :--- | :--- |
| **12** | **Nama:** | Hapus Kontak Darurat |
| | **URL** | `/api/contacts/:id` |
| | **Method** | `DELETE` |
| | **Type** | `Path Parameter` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `id` (string path, required: ID Kontak Darurat) |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `message` (string: "Kontak darurat berhasil dihapus")<br><br>**Gagal (400 Bad Request / 401 Unauthorized / 500 Server Error):**<br>- `error` (string: pesan kegagalan) |
| | **Keterangan** | Memutuskan hubungan kontak darurat secara permanen dengan pengguna lain. |
| :--- | :--- | :--- |
| **13** | **Nama:** | Ambil Permintaan Masuk |
| | **URL** | `/api/contacts/requests` |
| | **Method** | `GET` |
| | **Type** | `-` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `-` |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `requests` (array object: `id` (ID Hubungan), `name`, `phone_number`, `profile_image`, `status` [selalu 'Menunggu Konfirmasi'])<br><br>**Gagal (401 Unauthorized / 500 Server Error):**<br>- `error` (string: pesan kegagalan) |
| | **Keterangan** | Mengambil semua daftar permintaan kontak darurat masuk dari user lain yang menunggu persetujuan Anda. |
| :--- | :--- | :--- |
| **14** | **Nama:** | Terima Permintaan Kontak |
| | **URL** | `/api/contacts/requests/:id/accept` |
| | **Method** | `POST` |
| | **Type** | `Path Parameter` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `id` (string path, required: ID Hubungan Kontak) |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `message` (string: "Permintaan kontak darurat diterima")<br><br>**Gagal (400 Bad Request / 401 Unauthorized):**<br>- `error` (string: hubungan tidak ditemukan atau sudah diproses) |
| | **Keterangan** | Menerima permintaan kontak darurat yang dikirimkan oleh pengguna lain. |
| :--- | :--- | :--- |
| **15** | **Nama:** | Tolak/Hapus Permintaan Kontak |
| | **URL** | `/api/contacts/requests/:id/reject` |
| | **Method** | `POST` |
| | **Type** | `Path Parameter` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `id` (string path, required: ID Hubungan Kontak) |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `message` (string: "Permintaan kontak darurat ditolak")<br><br>**Gagal (400 Bad Request / 401 Unauthorized):**<br>- `error` (string: hubungan tidak ditemukan atau sudah diproses) |
| | **Keterangan** | Menolak atau menghapus hubungan permintaan kontak darurat dari pengguna lain. |
| :--- | :--- | :--- |
| **16** | **Nama:** | Trigger SOS / Kirim Sinyal Darurat |
| | **URL** | `/api/sos/trigger` |
| | **Method** | `POST` |
| | **Type** | `JSON` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `trigger_type` (string, required, "manual" atau "auto")<br>`latitude` (float64, required)<br>`longitude` (float64, required) |
| | **Return value** | **JSON**<br><br>**Success (201 Created / 200 OK jika sudah ada yang aktif):**<br>- `sos_id` (string)<br>- `user_id` (string)<br>- `trigger_type` (string)<br>- `status` (string, e.g. "active")<br>- `initial_latitude` (float64)<br>- `initial_longitude` (float64)<br>- `medical_snapshot` (object)<br>- `created_at` (string)<br><br>**Gagal (400 Bad Request / 401 Unauthorized / 500 Server Error):**<br>- `error` (string: pesan kegagalan) |
| | **Keterangan** | Digunakan untuk mengaktifkan mode darurat SOS (baik manual oleh user maupun otomatis via deteksi sensor). Sistem akan otomatis menyimpan snapshot rekam medis dan mengirimkan notifikasi push ke seluruh kontak darurat aktif. |
| :--- | :--- | :--- |
| **17** | **Nama:** | Ambil SOS Aktif Milik Sendiri |
| | **URL** | `/api/sos/active` |
| | **Method** | `GET` |
| | **Type** | `-` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `-` |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `active` (boolean)<br>- `sos_id` (string/null)<br>- `event` (object/null)<br><br>**Gagal (401 Unauthorized / 500 Server Error):**<br>- `error` (string: pesan kegagalan) |
| | **Keterangan** | Memeriksa apakah pengguna saat ini sedang memiliki kejadian SOS yang aktif berjalan. |
| :--- | :--- | :--- |
| **18** | **Nama:** | Selesaikan Status SOS |
| | **URL** | `/api/sos/:id/resolve` |
| | **Method** | `POST` |
| | **Type** | `JSON` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `id` (string path, required: ID SOS)<br>`status` (string, required: "resolved" atau "false_alarm") |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `message` (string: "Status SOS berhasil diperbarui")<br>- `status` (string)<br><br>**Gagal (400 Bad Request / 401 Unauthorized / 500 Server Error):**<br>- `error` (string: pesan kegagalan) |
| | **Keterangan** | Digunakan oleh korban untuk menyelesaikan status darurat SOS (selesai ditangani atau alarm palsu). |
| :--- | :--- | :--- |
| **19** | **Nama:** | Kirim Pelacakan Lokasi SOS |
| | **URL** | `/api/sos/:id/track` |
| | **Method** | `POST` |
| | **Type** | `JSON` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `id` (string path, required: ID SOS)<br>`latitude` (float64, required)<br>`longitude` (float64, required) |
| | **Return value** | **JSON**<br><br>**Success (201 Created):**<br>- `tracking_id` (string)<br>- `sos_id` (string)<br>- `latitude` (float64)<br>- `longitude` (float64)<br>- `recorded_at` (string)<br><br>**Gagal (400 Bad Request / 401 Unauthorized / 403 Forbidden / 500 Server Error):**<br>- `error` (string: pesan kegagalan / bukan pemilik SOS) |
| | **Keterangan** | Digunakan oleh aplikasi Flutter korban untuk terus mengirimkan koordinat GPS secara berkala selama SOS dalam status aktif agar bisa dilacak oleh responder. |
| :--- | :--- | :--- |
| **20** | **Nama:** | Kirim Sinyal Penerimaan (Acknowledge) SOS |
| | **URL** | `/api/sos/:id/acknowledge` |
| | **Method** | `POST` |
| | **Type** | `-` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `id` (string path, required: ID SOS) |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `message` (string: "Sinyal balik SOS berhasil terkirim")<br><br>**Gagal (400 Bad Request / 401 Unauthorized / 500 Server Error):**<br>- `error` (string: pesan kegagalan) |
| | **Keterangan** | Digunakan oleh kontak darurat (responder) untuk mengonfirmasi bahwa mereka telah melihat/menanggapi status bahaya korban. |
| :--- | :--- | :--- |
| **21** | **Nama:** | Ambil Detail SOS Lengkap |
| | **URL** | `/api/sos/:id` |
| | **Method** | `GET` |
| | **Type** | `-` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `id` (string path, required: ID SOS) |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `sos_id` (string)<br>- `user_id` (string)<br>- `user_name` (string)<br>- `user_phone` (string)<br>- `trigger_type` (string)<br>- `status` (string)<br>- `initial_latitude` (float64)<br>- `initial_longitude` (float64)<br>- `medical_snapshot` (object)<br>- `created_at` (string)<br>- `tracking_points` (array object: detail koordinat pelacakan)<br>- `responders` (array object: daftar penolong yang melakukan acknowledge)<br><br>**Gagal (400 Bad Request / 401 Unauthorized / 404 Not Found / 500 Server Error):**<br>- `error` (string: pesan kegagalan) |
| | **Keterangan** | Mengambil seluruh riwayat pelacakan, penolong, info medis, dan lokasi terakhir dari suatu insiden SOS spesifik. |
| :--- | :--- | :--- |
| **22** | **Nama:** | Ambil Riwayat SOS Terkirim (Oleh Diri Sendiri) |
| | **URL** | `/api/sos/history/sent` |
| | **Method** | `GET` |
| | **Type** | `-` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `-` |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- Array of object `SosEvent`<br><br>**Gagal (401 Unauthorized / 500 Server Error):**<br>- `error` (string: pesan kegagalan) |
| | **Keterangan** | Mengambil seluruh riwayat daftar alarm darurat SOS yang pernah di-trigger oleh user saat ini di masa lalu. |
| :--- | :--- | :--- |
| **23** | **Nama:** | Ambil Riwayat SOS Diterima (Dari Teman) |
| | **URL** | `/api/sos/history/received` |
| | **Method** | `GET` |
| | **Type** | `-` |
| | **Authentifikasi** | `Bearer JWT Token` |
| | **Parameters** | `-` |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- Array of object `SosEvent` (dari kontak darurat teman)<br><br>**Gagal (401 Unauthorized / 500 Server Error):**<br>- `error` (string: pesan kegagalan) |
| | **Keterangan** | Mengambil seluruh riwayat daftar alarm darurat SOS dari kontak darurat pengguna lain (teman) yang menunjuk user saat ini sebagai penolong. |
| :--- | :--- | :--- |
| **24** | **Nama:** | Cek Heartbeat Layanan |
| | **URL** | `/` |
| | **Method** | `GET` |
| | **Type** | `-` |
| | **Authentifikasi** | `-` |
| | **Parameters** | `-` |
| | **Return value** | **JSON**<br><br>**Success (200 OK):**<br>- `status` (string: "Safe Backend is Running Natively!")<br><br>**Gagal:**<br>- *Server unreachable* |
| | **Keterangan** | Endpoint dasar non-API untuk memastikan kontainer / server backend menyala sempurna. |
| :--- | :--- | :--- |
