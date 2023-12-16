package scanner

import (
	"io"
	"slices"
)

type Pos struct {
	Line int
	Col  int
}

type Scanner struct {
	reader io.Reader
	buf    []byte

	// The first unfilled position in buf
	// 0 <= fillIndex <= len(buf)
	fillIndex int

	// Current position in buf
	// 0 <= currentIndex <= fillIndex
	currentIndex int

	// Records lineno and colno of current position (from when the scanning
	// started)
	currentPos, prevPos Pos

	// Position in buf of the currently recorded token.
	// -1 means not recording a token
	// 0 means there may be token parts no longer in the buffer
	// tokenStartIndex <= currentIndex
	tokenStartIndex int

	// Parts of a token that no longer fit in the read buffer.
	tokenParts [][]byte

	err error

	// Tracks how many EOFs have been read.  This is required to make
	// Back() work after an EOF has been read.
	eofCount int
}

func NewScanner(reader io.Reader) *Scanner {
	return NewScannerSize(reader, defaultBufSize)
}

func NewScannerSize(reader io.Reader, size int) *Scanner {
	return &Scanner{
		reader:          reader,
		buf:             make([]byte, size),
		tokenStartIndex: -1,
		prevPos:         Pos{Line: -1},
	}
}

func (s *Scanner) fillBuf() {
	if s.fillIndex == len(s.buf) {
		var baseIndex int
		// If we are recording a token then we try to shift the buffer so the token
		// remains wholly in the buffer.
		if s.tokenStartIndex > 0 {
			baseIndex = s.tokenStartIndex
			s.tokenStartIndex = 0
		} else if s.currentIndex >= lookBackSize {
			baseIndex = s.currentIndex - lookBackSize
			if s.tokenStartIndex >= 0 {
				// At this point s.tokenStartIndex is 0
				newTokenBytes := make([]byte, baseIndex)
				copy(newTokenBytes, s.buf)
				s.tokenParts = append(s.tokenParts, newTokenBytes)
			}
		}
		if baseIndex > 0 {
			copy(s.buf, s.buf[baseIndex:s.fillIndex])
			s.fillIndex -= baseIndex
			s.currentIndex -= baseIndex
		}
	}
	for i := maxConsecutiveEmptyReads; i > 0; i-- {
		n, err := s.reader.Read(s.buf[s.fillIndex:])
		s.fillIndex += n
		if err != nil {
			s.err = err
			return
		}
		if n > 0 {
			return
		}
	}
	s.err = io.ErrNoProgress
}

func (s *Scanner) Read() (byte, error) {
	if s.currentIndex >= s.fillIndex {
		s.fillBuf()
	}
	if s.currentIndex < s.fillIndex {
		b := s.buf[s.currentIndex]
		s.prevPos = s.currentPos
		switch {
		case b == '\n':
			s.currentPos.Line++
			s.currentPos.Col = 0
		case b < 0xC0:
			// This is the last byte in an utf8-encoded codepoint
			s.currentPos.Col++
		}
		s.currentIndex++
		return b, nil
	}
	if s.err == io.EOF {
		s.eofCount++
		return EOF, nil
	}
	return 0, s.err
}

func (s *Scanner) StartToken() Pos {
	if s.tokenStartIndex >= 0 {
		panic("already in record mode")
	}
	s.tokenStartIndex = s.currentIndex
	return s.currentPos
}

func (s *Scanner) CurrentPos() Pos {
	return s.currentPos
}

func (s *Scanner) EndToken() []byte {
	if s.tokenStartIndex < 0 {
		panic("not in record mode")
	}
	if s.tokenParts == nil {
		tokBytes := slices.Clone(s.buf[s.tokenStartIndex:s.currentIndex])
		s.tokenStartIndex = -1
		return tokBytes
	}
	// Precalculate the size of the token so it doesn't have to be grown mid-concatenation
	tokLen := s.currentIndex - s.tokenStartIndex
	for _, p := range s.tokenParts {
		tokLen += len(p)
	}
	tokBytes := make([]byte, 0, tokLen)
	for _, c := range s.tokenParts {
		tokBytes = append(tokBytes, c...)
	}
	tokBytes = append(tokBytes, s.buf[s.tokenStartIndex:s.currentIndex]...)
	s.tokenStartIndex = -1
	s.tokenParts = nil
	return tokBytes
}

func (s *Scanner) Back() {
	if s.currentIndex <= 0 || s.currentIndex <= s.tokenStartIndex {
		panic("cannot go back from start")
	}
	if s.prevPos.Line < 0 {
		panic("cannot go back twice")
	}
	if s.eofCount > 0 {
		s.eofCount--
		return
	}
	s.currentIndex--
	s.currentPos = s.prevPos
	s.prevPos.Line = -1
}

func (s *Scanner) Peek() (byte, error) {
	if s.currentIndex >= s.fillIndex {
		s.fillBuf()
	}
	if s.currentIndex < s.fillIndex {
		return s.buf[s.currentIndex], nil
	}
	return s.errOrEOF()
}

func (s *Scanner) errOrEOF() (byte, error) {
	if s.err == io.EOF {
		return EOF, nil
	}
	return 0, s.err
}

func (s *Scanner) SkipSpaceAndPeek() (byte, error) {
	for {
		for i, b := range s.buf[s.currentIndex:s.fillIndex] {
			switch {
			case b == '\n':
				s.currentPos.Line++
				s.currentPos.Col = 0
			case b == ' ' || b == '\t' || b == '\r':
				s.currentPos.Col++
			default:
				s.currentIndex += i
				return b, nil
			}
		}
		s.currentIndex = s.fillIndex
		s.fillBuf()
		if s.currentIndex >= s.fillIndex {
			return s.errOrEOF()
		}
	}
}

func (s *Scanner) SkipSpaceAndRead() (byte, error) {
	for {
		for i, b := range s.buf[s.currentIndex:s.fillIndex] {
			switch {
			case b == '\n':
				s.currentPos.Line++
				s.currentPos.Col = 0
			case b == ' ' || b == '\t' || b == '\r':
				s.currentPos.Col++
			default:
				s.currentIndex += i + 1
				if b < 0xC0 {
					s.currentPos.Col++
				}
				return b, nil
			}
		}
		s.currentIndex = s.fillIndex
		s.fillBuf()
		if s.currentIndex >= s.fillIndex {
			return s.errOrEOF()
		}
	}
}

const (
	lookBackSize             = 1
	maxConsecutiveEmptyReads = 100
	defaultBufSize           = 8192
)

// 0xFF is a byte that should not appear in a UTF-8 encoded stream of bytes.
const EOF byte = 0xFF
