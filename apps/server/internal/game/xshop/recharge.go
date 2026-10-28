package xshop

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Layout of the 0x23f3-byte recharge block sent by FA 0F and received by
// FA 10.
const (
	RechargeSize    = 0x23f3
	RechargeActive  = 0x0000 // Ativo == 1
	RechargeHolder  = 0x0001 // Titular (0x60)
	RechargeKey     = 0x0061 // ChavePix (0x80)
	RechargeCopy    = 0x00e1 // CopiaECola (0x200)
	RechargeQRSize  = 0x02e1 // u16 matrix side (0..256)
	RechargeQRBits  = 0x02e3 // side*side bits, row major (0x2000)
	RechargeURL     = 0x22e3 // ImagemURL (0x100)
	RechargeValue   = 0x23e3 // Valor (0x10), from the EMV tag 54 of CopiaECola
	RechargeChunk   = 0xC0
	rechargeMaxSide = 0x100
)

// Recharge timing.
const (
	RechargeInterval  = 3000 * time.Millisecond  // FA 0E and the first FA 10
	RechargeUploadTTL = 10000 * time.Millisecond // 0x2711 > tick difference
)

// RechargeUpload is a GM upload in progress (the 0x2408-byte record of
// CCustomXShop +0x04).
type RechargeUpload struct {
	Account string
	At      time.Time
	Data    [RechargeSize]byte
	Filled  int
}

// PacketRechargeChunk is C1 C6 FA 0F offset(u16) data[0xC0] (FUN_00404a60);
// the last chunk is zero padded.
func PacketRechargeChunk(off int, d *[RechargeSize]byte) []byte {
	p := packet.New(frame.C1, 0xFA, 0x0F, 0xC6)
	packet.PutUint16(p[4:], off)
	copy(p[6:], d[off:min(off+RechargeChunk, RechargeSize)])
	return p
}

// PacketRechargeAck is C1 05 FA 11 (FUN_00404f90; the fifth byte is
// never written).
func PacketRechargeAck() []byte { return packet.New(frame.C1, 0xFA, 0x11, 5) }

// FieldText returns the NUL-terminated text of a fixed field.
func FieldText(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}

// IsHTTPURL is FUN_00401500.
func IsHTTPURL(s string) bool {
	l := strings.ToLower(s)
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")
}

// RechargeValid is FUN_00401140: Ativo 0 or 1; Titular, ChavePix,
// CopiaECola and ImagemURL NUL-terminated inside their fields without
// control characters or '"'; a matrix side of at most 256. Valor is then
// rebuilt from the EMV tag 54 of CopiaECola when it reads "digits.dd".
func RechargeValid(d *[RechargeSize]byte) bool {
	if d[RechargeActive] >= 2 {
		return false
	}
	for _, f := range [][2]int{{RechargeHolder, 0x60}, {RechargeKey, 0x80}, {RechargeCopy, 0x200}, {RechargeURL, 0x100}} {
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
	if binary.LittleEndian.Uint16(d[RechargeQRSize:]) > rechargeMaxSide {
		return false
	}
	d[RechargeValue] = 0
	if v, ok := emvAmount(FieldText(d[RechargeCopy : RechargeCopy+0x200])); ok && amountText(v) {
		clear(d[RechargeValue : RechargeValue+0x10])
		copy(d[RechargeValue:], v)
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

// LoadRechargeImage is FUN_00401bf0: the image (at most 8192x8192) is
// scaled into a side of min(max(width, height), 256), centred; a pixel is
// set when its colour over white sums to less than 0x180.
func LoadRechargeImage(path string, d *[RechargeSize]byte) bool {
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
	clear(d[RechargeQRBits : RechargeQRBits+0x2000])
	binary.LittleEndian.PutUint16(d[RechargeQRSize:], uint16(side))
	base := (side-dh)/2*side + (side-dw)/2
	const a = 0xff // GDI+ reads these formats as opaque
	over := func(c uint32) uint32 { return ((c-0xff)*a + 0xfe01) / 0xff }
	for y := 0; y < dh; y++ {
		sy := y * img.H / dh
		for x := 0; x < dw; x++ {
			r, g, bl := img.at(x*img.W/dw, sy)
			if over(r)+over(g)+over(bl) < 0x180 {
				bit := base + y*side + x
				d[RechargeQRBits+bit>>3] |= 1 << (bit & 7)
			}
		}
	}
	return true
}

// EncodeRechargeBMP is the image part of FUN_00405350: a top-down 32-bit
// bitmap of the matrix, black (0x00000000) for set bits, 0x00FFFFFF
// otherwise.
func EncodeRechargeBMP(d *[RechargeSize]byte) []byte {
	n := int(binary.LittleEndian.Uint16(d[RechargeQRSize:]))
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
		if d[RechargeQRBits+i>>3]&(1<<(i&7)) != 0 {
			v = 0
		}
		binary.LittleEndian.PutUint32(b[0x36+i*4:], v)
	}
	return b
}

// CutField is GetPrivateProfileStringA into a buffer of size bytes.
func CutField(s string, size int) string {
	if len(s) > size-1 {
		return s[:size-1]
	}
	return s
}
