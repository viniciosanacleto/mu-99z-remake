package game

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/ini"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// XShop recharge (CCustomXShop, 0x404a60 / 0x405030 / 0x405350): the PIX
// payment data of Custom/CustomXShopRecharge.txt that the client shows in
// the X Shop, with the QR code as a bit matrix. Players read it with FA 0E;
// a GM (Authority 8) replaces it with FA 10 uploads.
//
// Deliberate deviation: when ImagemBMP is missing or unreadable the
// original downloads ImagemURL (http/https, with its own user agent)
// and converts the PNG/JPG/GIF/BMP. This server never makes network
// requests: the matrix stays empty and the skip is logged (U-XSHOP-03).

func init() {
	faHandlers[0x0E] = sized(0x04, (*World).xshopRecharge)
	faHandlers[0x10] = sized(0xC6, (*World).xshopRechargeUpload)
}

// Layout of the 0x23f3-byte recharge block sent by FA 0F and received by
// FA 10.
const (
	rechargeSize    = 0x23f3
	rcActive        = 0x0000 // Ativo == 1
	rcHolder        = 0x0001 // Titular (0x60)
	rcKey           = 0x0061 // ChavePix (0x80)
	rcCopy          = 0x00e1 // CopiaECola (0x200)
	rcQRSize        = 0x02e1 // u16 matrix side (0..256)
	rcQRBits        = 0x02e3 // side*side bits, row major (0x2000)
	rcURL           = 0x22e3 // ImagemURL (0x100)
	rcValue         = 0x23e3 // Valor (0x10), from the EMV tag 54 of CopiaECola
	rechargeChunk   = 0xC0
	rechargeMaxSide = 0x100
)

const (
	rechargeInterval  = 3000 * time.Millisecond  // FA 0E and the first FA 10
	rechargeUploadTTL = 10000 * time.Millisecond // 0x2711 > tick difference
)

// rechargeUpload is a GM upload in progress (the 0x2408-byte record of
// CCustomXShop +0x04).
type rechargeUpload struct {
	Account string
	At      time.Time
	Data    [rechargeSize]byte
	Filled  int
}

// packetRechargeChunk is C1 C6 FA 0F offset(u16) data[0xC0] (FUN_00404a60);
// the last chunk is zero padded.
func packetRechargeChunk(off int, d *[rechargeSize]byte) []byte {
	p := newPacket(frame.C1, 0xFA, 0x0F, 0xC6)
	u16le(p[4:], off)
	copy(p[6:], d[off:min(off+rechargeChunk, rechargeSize)])
	return p
}

// packetRechargeAck is C1 05 FA 11 (FUN_00404f90; the fifth byte is
// never written).
func packetRechargeAck() []byte { return newPacket(frame.C1, 0xFA, 0x11, 5) }

func (w *World) rechargePath() string {
	c := w.cw2()
	if c.xshopPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(c.xshopPath), "CustomXShopRecharge.txt")
}

// fieldText returns the NUL-terminated text of a fixed field.
func fieldText(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}

// xshopRecharge is FUN_00404a60: at most every 3 s, the block is built from
// CustomXShopRecharge.txt (next to CustomXShop.txt) and sent in 48 chunks.
func (w *World) xshopRecharge(o *Object, p []byte) {
	if !playerReady(o) {
		return
	}
	s, now := cash(o), w.now()
	if !s.RechargeAt.IsZero() && now.Sub(s.RechargeAt) < rechargeInterval {
		return
	}
	s.RechargeAt = now
	path := w.rechargePath()
	if path == "" {
		return
	}
	d := w.rechargeBlock(path)
	for off := 0; off < rechargeSize; off += rechargeChunk {
		w.send(o, packetRechargeChunk(off, d))
	}
}

// rechargeBlock reads [Recarga] Ativo, Titular, ChavePix, CopiaECola,
// ImagemURL and ImagemBMP (a bare file name is taken from the directory of
// the file), converts the image (FUN_00401bf0) and validates the result
// (FUN_00401140): an invalid block is sent with Ativo 0.
func (w *World) rechargeBlock(path string) *[rechargeSize]byte {
	d := new([rechargeSize]byte)
	f, err := ini.Load(path)
	if err != nil {
		f = ini.Parse(nil)
	}
	if f.Int("Recarga", "Ativo", 0) == 1 {
		d[rcActive] = 1
	}
	copy(d[rcHolder:], f.String("Recarga", "Titular", "", 0x60))
	copy(d[rcKey:], f.String("Recarga", "ChavePix", "", 0x80))
	copy(d[rcCopy:], f.String("Recarga", "CopiaECola", "", 0x200))
	url := f.String("Recarga", "ImagemURL", "", 0x100)
	copy(d[rcURL:], url)
	img := f.String("Recarga", "ImagemBMP", "", 0x104)
	loaded := false
	if img != "" {
		if !strings.ContainsAny(img, `\/`) {
			img = filepath.Join(filepath.Dir(path), img)
		}
		if loaded = loadRechargeImage(img, d); !loaded {
			w.log.Warn("[CustomXShop][RECHARGE] Cache local invalido/ausente", "file", img)
		}
	}
	if !loaded && isHTTPURL(url) {
		w.log.Warn("[CustomXShop][RECHARGE] ImagemURL not downloaded (no network requests)", "url", url)
	}
	if !rechargeValid(d) {
		d[rcActive] = 0
	}
	return d
}

// isHTTPURL is FUN_00401500.
func isHTTPURL(s string) bool {
	l := strings.ToLower(s)
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")
}

// rechargeValid is FUN_00401140: Ativo 0 or 1; Titular, ChavePix,
// CopiaECola and ImagemURL NUL-terminated inside their fields without
// control characters or '"'; a matrix side of at most 256. Valor is then
// rebuilt from the EMV tag 54 of CopiaECola when it reads "digits.dd".
func rechargeValid(d *[rechargeSize]byte) bool {
	if d[rcActive] >= 2 {
		return false
	}
	for _, f := range [][2]int{{rcHolder, 0x60}, {rcKey, 0x80}, {rcCopy, 0x200}, {rcURL, 0x100}} {
		b := d[f[0] : f[0]+f[1]]
		end := bytes.IndexByte(b, 0)
		if end < 0 {
			return false
		}
		for _, c := range b[:end] {
			if c < 0x20 || c == '"' {
				return false
			}
		}
	}
	if binary.LittleEndian.Uint16(d[rcQRSize:]) > rechargeMaxSide {
		return false
	}
	d[rcValue] = 0
	if v, ok := emvAmount(fieldText(d[rcCopy : rcCopy+0x200])); ok && amountText(v) {
		clear(d[rcValue : rcValue+0x10])
		copy(d[rcValue:], v)
	}
	return true
}

// emvAmount is FUN_00401080: walk the "tag(2) length(2 digits) value"
// fields of the PIX copy-and-paste text and return the value of tag 54. A
// malformed field, a second tag 54 or a value of 16 bytes or more fail.
func emvAmount(s string) (string, bool) {
	var out string
	found := false
	for pos := 0; pos < len(s); {
		if pos+4 > len(s) {
			return "", false
		}
		c1, c2 := s[pos+2], s[pos+3]
		if c1 < '0' || c1 > '9' || c2 < '0' || c2 > '9' {
			return "", false
		}
		n := int(c1-'0')*10 + int(c2-'0')
		next := pos + 4 + n
		if next > len(s) {
			return "", false
		}
		if s[pos] == '5' && s[pos+1] == '4' {
			if found || n >= 16 {
				return "", false
			}
			out, found = s[pos+4:next], true
		}
		pos = next
	}
	return out, found
}

// amountText is the Valor test of FUN_00401140: at least 4 characters,
// '.' third from the end, digits elsewhere.
func amountText(v string) bool {
	if len(v) < 4 || v[len(v)-3] != '.' {
		return false
	}
	for i := 0; i < len(v); i++ {
		if i != len(v)-3 && (v[i] < '0' || v[i] > '9') {
			return false
		}
	}
	return true
}

// --- image -> bit matrix ------------------------------------------------------------

// bmpImage is an uncompressed 24/32-bit Windows bitmap.
type bmpImage struct {
	W, H    int
	bpp     int
	topDown bool
	stride  int
	pix     []byte
}

// decodeBMP reads BI_RGB (and 32-bit BI_BITFIELDS) bitmaps of 24 or 32
// bits; the original goes through GDI+, which also reads other formats
// (U-XSHOP-04). Alpha is ignored, as GDI+ does for these formats.
func decodeBMP(b []byte) (*bmpImage, error) {
	if len(b) < 54 || b[0] != 'B' || b[1] != 'M' {
		return nil, fmt.Errorf("not a BMP")
	}
	off := int(binary.LittleEndian.Uint32(b[10:]))
	w := int(int32(binary.LittleEndian.Uint32(b[18:])))
	h := int(int32(binary.LittleEndian.Uint32(b[22:])))
	bpp := int(binary.LittleEndian.Uint16(b[28:]))
	comp := binary.LittleEndian.Uint32(b[30:])
	if (bpp != 24 && bpp != 32) || (comp != 0 && !(comp == 3 && bpp == 32)) || w <= 0 || h == 0 {
		return nil, fmt.Errorf("unsupported BMP (%d bpp, compression %d)", bpp, comp)
	}
	img := &bmpImage{W: w, H: h, bpp: bpp}
	if h < 0 {
		img.H, img.topDown = -h, true
	}
	img.stride = (w*bpp + 31) / 32 * 4
	if off < 0 || off+img.stride*img.H > len(b) {
		return nil, fmt.Errorf("truncated BMP")
	}
	img.pix = b[off:]
	return img, nil
}

// at returns the RGB of (x, y), y counted from the top.
func (m *bmpImage) at(x, y int) (r, g, bl uint32) {
	row := y
	if !m.topDown {
		row = m.H - 1 - y
	}
	p := m.pix[row*m.stride+x*m.bpp/8:]
	return uint32(p[2]), uint32(p[1]), uint32(p[0])
}

// loadRechargeImage is FUN_00401bf0: the image (at most 8192x8192) is
// scaled into a side of min(max(width, height), 256), centred; a pixel is
// set when its colour over white sums to less than 0x180.
func loadRechargeImage(path string, d *[rechargeSize]byte) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	img, err := decodeBMP(b)
	if err != nil || img.W > 0x2000 || img.H > 0x2000 {
		return false
	}
	big := max(img.W, img.H)
	side := min(big, rechargeMaxSide)
	dw, dh := max(img.W*side/big, 1), max(img.H*side/big, 1)
	clear(d[rcQRBits : rcQRBits+0x2000])
	binary.LittleEndian.PutUint16(d[rcQRSize:], uint16(side))
	base := (side-dh)/2*side + (side-dw)/2
	const a = 0xff // GDI+ reads these formats as opaque
	over := func(c uint32) uint32 { return ((c-0xff)*a + 0xfe01) / 0xff }
	for y := 0; y < dh; y++ {
		sy := y * img.H / dh
		for x := 0; x < dw; x++ {
			r, g, bl := img.at(x*img.W/dw, sy)
			if over(r)+over(g)+over(bl) < 0x180 {
				bit := base + y*side + x
				d[rcQRBits+bit>>3] |= 1 << (bit & 7)
			}
		}
	}
	return true
}

// encodeRechargeBMP is the image part of FUN_00405350: a top-down 32-bit
// bitmap of the matrix, black (0x00000000) for set bits, 0x00FFFFFF
// otherwise.
func encodeRechargeBMP(d *[rechargeSize]byte) []byte {
	n := int(binary.LittleEndian.Uint16(d[rcQRSize:]))
	b := make([]byte, 0x36+n*n*4)
	b[0], b[1] = 'B', 'M'
	binary.LittleEndian.PutUint32(b[2:], uint32(len(b)))
	binary.LittleEndian.PutUint32(b[10:], 0x36)
	binary.LittleEndian.PutUint32(b[14:], 0x28)
	binary.LittleEndian.PutUint32(b[18:], uint32(n))
	binary.LittleEndian.PutUint32(b[22:], uint32(-int32(n)))
	binary.LittleEndian.PutUint16(b[26:], 1)
	binary.LittleEndian.PutUint16(b[28:], 32)
	for i := 0; i < n*n; i++ {
		v := uint32(0xffffff)
		if d[rcQRBits+i>>3]&(1<<(i&7)) != 0 {
			v = 0
		}
		binary.LittleEndian.PutUint32(b[0x36+i*4:], v)
	}
	return b
}

// --- FA 10: GM upload ---------------------------------------------------------------

// xshopRechargeUpload is FUN_00405030: [4] offset(u16) [6] 0xC0 bytes.
// Only Authority 8. Offset 0 opens a new upload (at most every 3 s); the
// chunks must follow in order from the same account within 10 s. The
// complete block is validated and saved (FUN_00405350). FA 11 answers a
// refused, broken or finished upload, never an accepted middle chunk.
func (w *World) xshopRechargeUpload(o *Object, p []byte) {
	if !playerReady(o) {
		return
	}
	if mv(o).Authority != 8 {
		w.send(o, packetRechargeAck())
		return
	}
	c, s, now := w.cw2(), cash(o), w.now()
	login := o.P.Account.Login
	off := int(binary.LittleEndian.Uint16(p[4:]))
	if off == 0 {
		if !s.UploadAt.IsZero() && now.Sub(s.UploadAt) < rechargeInterval {
			w.send(o, packetRechargeAck())
			return
		}
		s.UploadAt = now
		c.uploads[o.Index] = &rechargeUpload{Account: login, At: now}
	}
	u := c.uploads[o.Index]
	if u == nil {
		return
	}
	if now.Sub(u.At) > rechargeUploadTTL || u.Account != login || off != u.Filled || off >= rechargeSize {
		delete(c.uploads, o.Index)
		w.send(o, packetRechargeAck())
		return
	}
	n := min(rechargeChunk, rechargeSize-off)
	copy(u.Data[off:off+n], p[6:6+n])
	u.Filled += n
	if u.Filled != rechargeSize {
		return
	}
	delete(c.uploads, o.Index)
	if rechargeValid(&u.Data) && w.rechargeSave(&u.Data) {
		w.log.Info("[CustomXShop][RECHARGE][SAVED]", "account", login, "name", o.P.Char.Name)
	}
	w.send(o, packetRechargeAck())
}

// rechargeSave is FUN_00405350: the matrix goes to a new "pix*.tmp.bmp"
// next to the file (when it has a side) and the [Recarga] file is
// rewritten through a temporary file. Old images are not removed.
func (w *World) rechargeSave(d *[rechargeSize]byte) bool {
	c := w.cw2()
	path := w.rechargePath()
	if c.saving || path == "" {
		return false
	}
	c.saving = true
	defer func() { c.saving = false }()
	dir := filepath.Dir(path)
	bmpName := ""
	if binary.LittleEndian.Uint16(d[rcQRSize:]) != 0 {
		f, err := os.CreateTemp(dir, "pix*.tmp")
		if err != nil {
			return false
		}
		tmp := f.Name()
		_, err = f.Write(encodeRechargeBMP(d))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err == nil {
			err = os.Rename(tmp, tmp+".bmp")
		}
		if err != nil {
			os.Remove(tmp)
			return false
		}
		bmpName = tmp + ".bmp"
	}
	fail := func(tmp string) bool {
		os.Remove(tmp)
		if bmpName != "" {
			os.Remove(bmpName)
		}
		return false
	}
	f, err := os.CreateTemp(dir, "cfg*.tmp")
	if err != nil {
		return fail("")
	}
	tmp := f.Name()
	imgBase := ""
	if bmpName != "" {
		imgBase = filepath.Base(bmpName)
	}
	_, err = fmt.Fprintf(f, "; Configurado pelo GM no X Shop. Credito manual.\r\n[Recarga]\r\nAtivo=%d\r\n"+
		"Titular=%s\r\nChavePix=%s\r\nCopiaECola=%s\r\nImagemURL=%s\r\nImagemBMP=%s\r\nValor=%s\r\n",
		d[rcActive], fieldText(d[rcHolder:rcHolder+0x60]), fieldText(d[rcKey:rcKey+0x80]), fieldText(d[rcCopy:rcCopy+0x200]),
		fieldText(d[rcURL:rcURL+0x100]), imgBase, fieldText(d[rcValue:rcValue+0x10]))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil || os.Rename(tmp, path) != nil {
		return fail(tmp)
	}
	return true
}
