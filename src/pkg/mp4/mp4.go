package mp4

import (
	"fmt"
	"os"
	"encoding/binary"
)

const (
	BOX_HEADER_SIZE = int64(8)
)

func Open(path string) (f *File, err os.Error) {
	// fmt.Println(flag.Args())
	fmt.Println(path)

	file, err := os.Open(path, os.O_RDONLY, 0400)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return nil, err
	}

	f = &File{
		File: file,
	}

	return f, f.parse()
}

func (f *File) parse() (os.Error) {
	info, err := f.Stat()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	fmt.Printf("File size: %v \n", info.Size)
	f.size = info.Size

	// Loop through top-level Boxes
	boxes := readBoxes(f, int64(0), f.size)
	for box := range boxes {
		switch box.Name() {
		case "ftyp":
			f.ftyp = &FtypBox{ Box:box }
			f.ftyp.parse()
		case "moov":
			f.moov = &MoovBox{ Box:box }
			f.moov.parse()
		case "mdat":
			f.mdat = box
		default:
			fmt.Printf("Unhandled Box: %v \n", box.Name())
		}
	}
	return nil
}

func readBoxes(f *File, start int64, n int64) (boxes chan *Box) {
	boxes = make(chan *Box, 100)
	go func() {
		for offset := start; offset < start + n; {
			size, name := f.ReadBoxAt(offset)
			fmt.Printf("Box found:\nType: %v \nSize (bytes): %v \n", name, size)

			box := &Box {
				name:		name,
				size:		int64(size),
				start:	offset,
				file:		f,
			}
			boxes <- box
			offset += int64(size)
		}
		close(boxes)
	} ()
	return boxes
}

func readSubBoxes(f *File, start int64, n int64) (boxes chan *Box) {
	return readBoxes(f, start + BOX_HEADER_SIZE, n - BOX_HEADER_SIZE)
}

type File struct {
	*os.File
	ftyp *FtypBox
	moov *MoovBox
	mdat *Box
	size int64
}

func (f *File) ReadBoxAt(offset int64) (boxSize uint32, boxType string) {
	// Get Box size
	buf := f.ReadBytesAt(BOX_HEADER_SIZE, offset)
	boxSize = binary.BigEndian.Uint32(buf[0:4])
	offset += BOX_HEADER_SIZE
	// Get Box name
	boxType = string(buf[4:8])
	return boxSize, boxType
}

func (f *File) ReadBytesAt(n int64, offset int64) (word []byte) {
	buf := make([]byte, n)
	_, error := f.ReadAt(buf, offset)
	if error != nil {
		fmt.Println(error)
		return
	}
	return buf
}

type BoxInt interface {
	Name() string
	File() *File
	Size() int64
	Start() int64
	parse() os.Error
}

type Box struct {
	name string
	size, start int64
	file *File
}

func (b *Box) Name() (string) { return b.name }

func (b *Box) Size() (int64) { return b.size }

func (b *Box) File() (*File) { return b.file }

func (b *Box) Start() (int64) { return b.start }

func (b *Box) parse() (os.Error) {
	fmt.Printf("Default parser called; skip parsing. (%v)\n", b.name)
	return nil
}

func (b *Box) ReadBoxData() ([]byte) {
	if b.Size() <= BOX_HEADER_SIZE {
		return nil
	}
	return b.File().ReadBytesAt(b.Size() - BOX_HEADER_SIZE, b.Start() + BOX_HEADER_SIZE)
}

type FtypBox struct {
	*Box
	major_brand, minor_version string
	compatible_brands []string
}

func (b *FtypBox) parse() (os.Error) {
	data := b.ReadBoxData()
	b.major_brand, b.minor_version = string(data[0:4]), string(data[4:8])
	if len(data) > 8 {
		for i := 8; i < len(data); i += 4 {
			b.compatible_brands = append(b.compatible_brands, string(data[i:i+4]))
		}
	}
	return nil
}

type MoovBox struct {
	*Box
	mvhd *MvhdBox
	iods *IodsBox
	traks []*TrakBox
}

func (b *MoovBox) parse() (os.Error) {
	boxes := readSubBoxes(b.File(), b.Start(), b.Size())
	for subBox := range boxes {
		switch subBox.Name() {
		case "mvhd":
			b.mvhd = &MvhdBox{ Box:subBox }
			b.mvhd.parse()
		case "iods":
			b.iods = &IodsBox{ Box:subBox }
			b.iods.parse()
		case "trak":
			trak := &TrakBox{ Box:subBox }
			trak.parse()
			b.traks = append(b.traks, trak)
		default:
			fmt.Printf("Unhandled Moov Sub-Box: %v \n", subBox.Name())
		}
	}
	return nil
}

type MvhdBox struct {
	*Box
	version uint8
	flags [3]byte
	creation_time, modification_time, timescale, duration, next_track_id uint32
	rate Fixed32
	volume Fixed16
	other_data []byte
}

func (b *MvhdBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{data[1], data[2], data[3]}
	b.creation_time = binary.BigEndian.Uint32(data[4:8])
	b.modification_time = binary.BigEndian.Uint32(data[8:12])
	b.timescale = binary.BigEndian.Uint32(data[12:16])
	b.duration = binary.BigEndian.Uint32(data[16:20])
	b.rate, err = MakeFixed32(data[20:24])
	if err != nil {
		return err
	}
	b.volume, err = MakeFixed16(data[24:26])
	if err != nil {
		return err
	}
	b.other_data = data[26:]
	return nil
}

type IodsBox struct {
	*Box
	data []byte
}

func (b *IodsBox) parse() (os.Error) {
	b.data = b.ReadBoxData()
	return nil
}

type TrakBox struct {
	*Box
	tkhd *TkhdBox
	mdia *MdiaBox
}

func (b *TrakBox) parse() (os.Error) {
	boxes := readSubBoxes(b.File(), b.Start(), b.Size())
	for subBox := range boxes {
		switch subBox.Name() {
		case "tkhd":
			b.tkhd = &TkhdBox{ Box:subBox }
			b.tkhd.parse()
		case "mdia":
			b.mdia = &MdiaBox{ Box:subBox }
			b.mdia.parse()
		default:
			fmt.Printf("Unhandled Trak Sub-Box: %v \n", subBox.Name())
		}
	}
	return nil
}

type TkhdBox struct {
	*Box
	version uint8
	flags [3]byte
	creation_time, modification_time, track_id, duration uint32
	layer, alternate_group uint16 // This should really be int16 but not sure how to parse
	volume Fixed16
	matrix []byte
	width, height Fixed32
}

func (b *TkhdBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.creation_time = binary.BigEndian.Uint32(data[4:8])
	b.modification_time = binary.BigEndian.Uint32(data[8:12])
	b.track_id = binary.BigEndian.Uint32(data[12:16])
	// Skip 4 bytes for reserved space (uint32)
	b.duration = binary.BigEndian.Uint32(data[20:24])
	// Skip 8 bytes for reserved space (2 uint32)
	b.layer = binary.BigEndian.Uint16(data[32:34])
	b.alternate_group = binary.BigEndian.Uint16(data[34:36])
	b.volume, err = MakeFixed16(data[36:38])
	if err != nil {
		return err
	}
	// Skip 2 bytes for reserved space (uint16)
	b.matrix = data[40:76]
	b.width, err = MakeFixed32(data[76:80])
	if err != nil {
		return err
	}
	b.height, err = MakeFixed32(data[80:84])
	if err != nil {
		return err
	}
	return nil
}

type MdiaBox struct {
	*Box
	mdhd *MdhdBox
	hdlr *HdlrBox
	minf *MinfBox
}

func (b *MdiaBox) parse() (os.Error) {
	boxes := readSubBoxes(b.File(), b.Start(), b.Size())
	for subBox := range boxes {
		switch subBox.Name() {
		case "mdhd":
			b.mdhd = &MdhdBox{ Box:subBox }
			b.mdhd.parse()
		case "hdlr":
			b.hdlr = &HdlrBox{ Box:subBox }
			b.hdlr.parse()
		case "minf":
			b.minf = &MinfBox{ Box:subBox }
			b.minf.parse()
		default:
			fmt.Printf("Unhandled Mdia Sub-Box: %v \n", subBox.Name())
		}
	}
	return nil
}

type MdhdBox struct {
	*Box
	version uint8
	flags [3]byte
	creation_time, modification_time, timescale, duration uint32
	language uint16 // Combine 1-bit padding w/ 15-bit language data
}

func (b *MdhdBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.creation_time = binary.BigEndian.Uint32(data[4:8])
	b.modification_time = binary.BigEndian.Uint32(data[8:12])
	b.timescale = binary.BigEndian.Uint32(data[12:16])
	b.duration = binary.BigEndian.Uint32(data[16:20])
	// language includes 1 padding bit
	b.language = binary.BigEndian.Uint16(data[20:22])
	return nil
}

type HdlrBox struct {
	*Box
	version uint8
	flags [3]byte
	pre_defined uint32
	handler_type, track_name string
}

func (b *HdlrBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.pre_defined = binary.BigEndian.Uint32(data[4:8])
	b.handler_type = string(data[8:12])
	// Skip 12 bytes for reserved space (3 uint32)
	b.track_name = string(data[24:])
	return nil
}

type MinfBox struct {
	*Box
	vmhd *VmhdBox
	smhd *SmhdBox
	stbl *StblBox
}

func (b *MinfBox) parse() (err os.Error) {
	boxes := readSubBoxes(b.File(), b.Start(), b.Size())
	for subBox := range boxes {
		switch subBox.Name() {
		case "vmhd":
			b.vmhd = &VmhdBox{ Box:subBox }
			err = b.vmhd.parse()
		case "smhd":
			b.smhd = &SmhdBox{ Box:subBox }
			err = b.smhd.parse()
		case "stbl":
			b.stbl = &StblBox{ Box:subBox }
			err = b.stbl.parse()
		default:
			fmt.Printf("Unhandled Minf Sub-Box: %v \n", subBox.Name())
		}
		if err != nil {
			return err
		}
	}
	return nil
}

type VmhdBox struct {
	*Box
	version uint8
	flags [3]byte
	graphicsmode uint16
	opcolor [3]uint16
}

func (b *VmhdBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.graphicsmode = binary.BigEndian.Uint16(data[4:6])
	for i := 0; i < 3; i++ {
		b.opcolor[i] = binary.BigEndian.Uint16(data[(6+2*i):(8+2*i)])
	}
	return nil
}

type SmhdBox struct {
	*Box
	version uint8
	flags [3]byte
	balance uint16 // This should really be int16 but not sure how to parse
}

func (b *SmhdBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.balance = binary.BigEndian.Uint16(data[4:6])
	return nil
}

type StblBox struct {
	*Box
	stsd *StsdBox
}

func (b *StblBox) parse() (err os.Error) {
	boxes := readSubBoxes(b.File(), b.Start(), b.Size())
	for subBox := range boxes {
		switch subBox.Name() {
		case "stsd":
			b.stsd = &StsdBox{ Box:subBox }
			err = b.stsd.parse()
		default:
			fmt.Printf("Unhandled Stbl Sub-Box: %v \n", subBox.Name())
		}
		if err != nil {
			return err
		}
	}
	return nil
}

type StsdBox struct {
	*Box
	version uint8
	flags [3]byte
	entry_count uint32
	other_data []byte
}

func (b *StsdBox) parse() (err os.Error) {
	data := b.ReadBoxData()
	b.version = data[0]
	b.flags = [3]byte{ data[1], data[2], data[3] }
	b.entry_count = binary.BigEndian.Uint32(data[4:8])
	b.other_data = data[8:]
	fmt.Println("stsd box parsing not yet finished")
	return nil
}

// An 8.8 Fixed Point Decimal notation
type Fixed16 uint16

func (f Fixed16) String() string {
	return fmt.Sprintf("%v", uint16(f) >> 8)
}

func MakeFixed16(bytes []byte) (Fixed16, os.Error) {
	if len(bytes) != 2 {
		return Fixed16(0), os.NewError("Invalid number of bytes for Fixed16. Need 2, got " + string(len(bytes)))
	}
	return Fixed16(binary.BigEndian.Uint16(bytes)), nil
}

// A 16.16 Fixed Point Decimal notation
type Fixed32 uint32

func (f Fixed32) String() string {
	return fmt.Sprintf("%v", uint32(f) >> 16)
}

func MakeFixed32(bytes []byte) (Fixed32, os.Error) {
	if len(bytes) != 4 {
		return Fixed32(0), os.NewError("Invalid number of bytes for Fixed32. Need 4, got " + string(len(bytes)))
	}
	return Fixed32(binary.BigEndian.Uint32(bytes)), nil
}

type ContainerBox interface {
	ReadSubBoxes() (n int, err os.Error)
	HandleSubBox() (*Box, func(*Box))
}

type LeafBox interface {
	ReadData() (n int, err os.Error)
	ParseData() (n int, err os.Error)
	ReadAndParseData() (n int, err os.Error)
}
