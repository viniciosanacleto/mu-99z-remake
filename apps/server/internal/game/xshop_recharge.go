package game

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/xshop"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// XShop recharge (CCustomXShop, 0x404a60 / 0x405030 / 0x405350): the PIX
// payment data of CustomXShopRecharge.txt (the CustomXShopRecharge table of
// config/data/xshop.toml) that the client shows in the X Shop, with the QR
// code as a bit matrix (a BMP of data/Data/Custom). Players read it with FA 0E;
// a GM (Authority 8) replaces it with FA 10 uploads.
//
// Deliberate deviation: when ImagemBMP is missing or unreadable the
// original downloads ImagemURL (http/https, with its own user agent)
// and converts the PNG/JPG/GIF/BMP. This server never makes network
// requests: the matrix stays empty and the skip is logged (U-XSHOP-03).

func registerXshopRecharge(h *hooks) {
	h.faHandlers[0x0E] = sized(0x04, (*World).xshopRecharge)
	h.faHandlers[0x10] = sized(0xC6, (*World).xshopRechargeUpload)
}

// rechargePath is the xshop.toml holding the CustomXShopRecharge table.
func (w *World) rechargePath() string { return w.cw2().xshopPath }

// rechargeImageDir is the folder of the QR images (bare ImagemBMP names).
func (w *World) rechargeImageDir() string { return filepath.Join(w.dataDir, "Custom") }

// xshopRecharge is FUN_00404a60: at most every 3 s, the block is built from
// the CustomXShopRecharge table and sent in 48 chunks.
func (w *World) xshopRecharge(o *Object, p []byte) {
	if !playerReady(o) {
		return
	}
	s, now := cash(o), w.now()
	if !s.RechargeAt.IsZero() && now.Sub(s.RechargeAt) < xshop.RechargeInterval {
		return
	}
	s.RechargeAt = now
	path := w.rechargePath()
	if path == "" {
		return
	}
	d := w.rechargeBlock(path)
	for off := 0; off < xshop.RechargeSize; off += xshop.RechargeChunk {
		w.send(o, xshop.PacketRechargeChunk(off, d))
	}
}

// rechargeBlock reads the CustomXShopRecharge table ([Recarga] of the
// original: Ativo, Titular, ChavePix, CopiaECola, ImagemURL and ImagemBMP,
// a bare file name being taken from data/Data/Custom), converts the image
// (FUN_00401bf0) and validates the result (FUN_00401140): an invalid block
// is sent with Ativo 0.
func (w *World) rechargeBlock(path string) *[xshop.RechargeSize]byte {
	d := new([xshop.RechargeSize]byte)
	var r gamedata.XShopRechargeFile
	if f, err := gamedata.LoadXShopConfig(path); err == nil {
		r = f.CustomXShopRecharge
	}
	if r.Ativo == 1 {
		d[xshop.RechargeActive] = 1
	}
	copy(d[xshop.RechargeHolder:], xshop.CutField(r.Titular, 0x60))
	copy(d[xshop.RechargeKey:], xshop.CutField(r.ChavePix, 0x80))
	copy(d[xshop.RechargeCopy:], xshop.CutField(r.CopiaECola, 0x200))
	url := xshop.CutField(r.ImagemURL, 0x100)
	copy(d[xshop.RechargeURL:], url)
	img := xshop.CutField(r.ImagemBMP, 0x104)
	loaded := false
	if img != "" {
		if !strings.ContainsAny(img, `\/`) {
			img = filepath.Join(w.rechargeImageDir(), img)
		}
		if loaded = xshop.LoadRechargeImage(img, d); !loaded {
			w.log.Warn("[CustomXShop][RECHARGE] Cache local invalido/ausente", "file", img)
		}
	}
	if !loaded && xshop.IsHTTPURL(url) {
		w.log.Warn("[CustomXShop][RECHARGE] ImagemURL not downloaded (no network requests)", "url", url)
	}
	if !xshop.RechargeValid(d) {
		d[xshop.RechargeActive] = 0
	}
	return d
}

// --- image -> bit matrix ------------------------------------------------------------

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
	if !gmFull(o) {
		w.send(o, xshop.PacketRechargeAck())
		return
	}
	c, s, now := w.cw2(), cash(o), w.now()
	login := o.P.Account.Login
	off := int(binary.LittleEndian.Uint16(p[4:]))
	if off == 0 {
		if !s.UploadAt.IsZero() && now.Sub(s.UploadAt) < xshop.RechargeInterval {
			w.send(o, xshop.PacketRechargeAck())
			return
		}
		s.UploadAt = now
		c.uploads[o.Index] = &xshop.RechargeUpload{Account: login, At: now}
	}
	u := c.uploads[o.Index]
	if u == nil {
		return
	}
	if now.Sub(u.At) > xshop.RechargeUploadTTL || u.Account != login || off != u.Filled || off >= xshop.RechargeSize {
		delete(c.uploads, o.Index)
		w.send(o, xshop.PacketRechargeAck())
		return
	}
	n := min(xshop.RechargeChunk, xshop.RechargeSize-off)
	copy(u.Data[off:off+n], p[6:6+n])
	u.Filled += n
	if u.Filled != xshop.RechargeSize {
		return
	}
	delete(c.uploads, o.Index)
	if xshop.RechargeValid(&u.Data) && w.rechargeSave(&u.Data) {
		w.log.Info("[CustomXShop][RECHARGE][SAVED]", "account", login, "name", o.P.Char.Name)
	}
	w.send(o, xshop.PacketRechargeAck())
}

// rechargeSave is FUN_00405350: the matrix goes to a new "pix*.tmp.bmp" in
// data/Data/Custom (when it has a side) and the CustomXShopRecharge table
// is rewritten (gamedata.WriteXShopRecharge). Old images are not removed.
func (w *World) rechargeSave(d *[xshop.RechargeSize]byte) bool {
	c := w.cw2()
	path := w.rechargePath()
	if c.saving || path == "" {
		return false
	}
	c.saving = true
	defer func() { c.saving = false }()
	dir := w.rechargeImageDir()
	bmpName := ""
	if binary.LittleEndian.Uint16(d[xshop.RechargeQRSize:]) != 0 {
		f, err := os.CreateTemp(dir, "pix*.tmp")
		if err != nil {
			return false
		}
		tmp := f.Name()
		_, err = f.Write(xshop.EncodeRechargeBMP(d))
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
	imgBase := ""
	if bmpName != "" {
		imgBase = filepath.Base(bmpName)
	}
	err := gamedata.WriteXShopRecharge(path, gamedata.XShopRechargeFile{Ativo: int(d[xshop.RechargeActive]),
		Titular: xshop.FieldText(d[xshop.RechargeHolder : xshop.RechargeHolder+0x60]), ChavePix: xshop.FieldText(d[xshop.RechargeKey : xshop.RechargeKey+0x80]),
		CopiaECola: xshop.FieldText(d[xshop.RechargeCopy : xshop.RechargeCopy+0x200]), ImagemURL: xshop.FieldText(d[xshop.RechargeURL : xshop.RechargeURL+0x100]),
		ImagemBMP: imgBase, Valor: xshop.FieldText(d[xshop.RechargeValue : xshop.RechargeValue+0x10])})
	if err != nil {
		if bmpName != "" {
			os.Remove(bmpName)
		}
		return false
	}
	return true
}
