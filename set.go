// Copyright 2018 Google LLC. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nftables

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/nftables/expr"

	"github.com/google/nftables/binaryutil"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

// SetConcatTypeBits defines concatination bits, originally defined in
// https://git.netfilter.org/iptables/tree/iptables/nft.c?id=26753888720d8e7eb422ae4311348347f5a05cb4#n1002
const (
	SetConcatTypeBits = 6
	SetConcatTypeMask = (1 << SetConcatTypeBits) - 1
)

var allocSetID uint32

// SetDatatype represents a datatype declared by nft.
type SetDatatype struct {
	Name  string
	Bytes uint32

	// nftMagic represents the magic value that nft uses for
	// certain types (ie: IP addresses). We populate SET_KEY_TYPE
	// identically, so `nft list ...` commands produce correct output.
	nftMagic uint32
}

// GetNFTMagic returns a custom datatype based on user's parameters
func (s *SetDatatype) GetNFTMagic() uint32 {
	return s.nftMagic
}

// SetNFTMagic returns a custom datatype based on user's parameters
func (s *SetDatatype) SetNFTMagic(nftMagic uint32) {
	s.nftMagic = nftMagic
}

// NFT datatypes. See: https://git.netfilter.org/nftables/tree/src/datatype.c
var (
	TypeInvalid     = SetDatatype{Name: "invalid", nftMagic: 0}
	TypeVerdict     = SetDatatype{Name: "verdict", Bytes: 0, nftMagic: 1}
	TypeNFProto     = SetDatatype{Name: "nfproto", Bytes: 1, nftMagic: 2}
	TypeBitmask     = SetDatatype{Name: "bitmask", nftMagic: 3} // TODO
	TypeInteger     = SetDatatype{Name: "integer", Bytes: 4, nftMagic: 4}
	TypeString      = SetDatatype{Name: "string", nftMagic: 5} // TODO
	TypeLLAddr      = SetDatatype{Name: "lladdr", nftMagic: 6} // TODO
	TypeIPAddr      = SetDatatype{Name: "ipv4_addr", Bytes: 4, nftMagic: 7}
	TypeIP6Addr     = SetDatatype{Name: "ipv6_addr", Bytes: 16, nftMagic: 8}
	TypeEtherAddr   = SetDatatype{Name: "ether_addr", Bytes: 6, nftMagic: 9}
	TypeEtherType   = SetDatatype{Name: "ether_type", nftMagic: 10} // TODO
	TypeARPOP       = SetDatatype{Name: "arp_op", nftMagic: 11}     // TODO
	TypeInetProto   = SetDatatype{Name: "inet_proto", Bytes: 1, nftMagic: 12}
	TypeInetService = SetDatatype{Name: "inet_service", Bytes: 2, nftMagic: 13}
	TypeICMPType    = SetDatatype{Name: "icmp_type", nftMagic: 14}     // TODO
	TypeTCPFlag     = SetDatatype{Name: "tcp_flag", nftMagic: 15}      // TODO
	TypeDCCPPktType = SetDatatype{Name: "dccp_pkt_type", nftMagic: 16} // TODO
	TypeMHType      = SetDatatype{Name: "mh_type", Bytes: 1, nftMagic: 17}
	TypeTime        = SetDatatype{Name: "time", Bytes: 8, nftMagic: 18}
	TypeMark        = SetDatatype{Name: "mark", Bytes: 4, nftMagic: 19}
	TypeIFIndex     = SetDatatype{Name: "if_index", nftMagic: 20} // TODO
	TypeARPHRD      = SetDatatype{Name: "arp_hrd", nftMagic: 21}  // TODO
	TypeRealm       = SetDatatype{Name: "realm", nftMagic: 22}    // TODO
	TypeClassID     = SetDatatype{Name: "class_id", nftMagic: 23} // TODO
	TypeUID         = SetDatatype{Name: "uid", nftMagic: 24}      // TODO
	TypeGID         = SetDatatype{Name: "gid", nftMagic: 25}      // TODO
	TypeCTState     = SetDatatype{Name: "ct_state", Bytes: 4, nftMagic: 26}
	TypeCTDir       = SetDatatype{Name: "ct_dir", Bytes: 1, nftMagic: 27}
	TypeCTStatus    = SetDatatype{Name: "ct_status", Bytes: 4, nftMagic: 28}
	TypeICMP6Type   = SetDatatype{Name: "icmp6_type", nftMagic: 29} // TODO
	TypePktType     = SetDatatype{Name: "pkt_type", nftMagic: 30}   // TODO
	TypeICMPCode    = SetDatatype{Name: "icmp_code", Bytes: 1, nftMagic: 31}
	TypeICMPv6Code  = SetDatatype{Name: "icmpv6_code", Bytes: 1, nftMagic: 32}
	TypeICMPxCode   = SetDatatype{Name: "icmpx_code", Bytes: 1, nftMagic: 33}
	TypeDevGroup    = SetDatatype{Name: "dev_group", nftMagic: 34}         // TODO
	TypeDSCP        = SetDatatype{Name: "dscp", nftMagic: 35}              // TODO
	TypeECN         = SetDatatype{Name: "ecn", nftMagic: 36}               // TODO
	TypeFIBAddr     = SetDatatype{Name: "fib_addr", nftMagic: 37}          // TODO
	TypeBoolean     = SetDatatype{Name: "boolean", Bytes: 1, nftMagic: 38} // Size is actually only 1 bit
	TypeIFName      = SetDatatype{Name: "if_name", nftMagic: 39}           // TODO
	TypeIGMPType    = SetDatatype{Name: "igmp_type", nftMagic: 40}         // TODO
	TypeTimeDate    = SetDatatype{Name: "time_date", nftMagic: 41}         // TODO
	TypeTimeHour    = SetDatatype{Name: "time_hour", nftMagic: 42}         // TODO
	TypeTimeDay     = SetDatatype{Name: "time_day", nftMagic: 43}          // TODO

	nftDatatypes = []SetDatatype{
		TypeVerdict,
		TypeNFProto,
		TypeBitmask,
		TypeInteger,
		TypeString,
		TypeLLAddr,
		TypeIPAddr,
		TypeIP6Addr,
		TypeEtherAddr,
		TypeEtherType,
		TypeARPOP,
		TypeInetProto,
		TypeInetService,
		TypeICMPType,
		TypeTCPFlag,
		TypeDCCPPktType,
		TypeMHType,
		TypeTime,
		TypeMark,
		TypeIFIndex,
		TypeARPHRD,
		TypeRealm,
		TypeClassID,
		TypeUID,
		TypeGID,
		TypeCTState,
		TypeCTDir,
		TypeCTStatus,
		TypeICMP6Type,
		TypePktType,
		TypeICMPCode,
		TypeICMPv6Code,
		TypeICMPxCode,
		TypeDevGroup,
		TypeDSCP,
		TypeECN,
		TypeFIBAddr,
		TypeBoolean,
		TypeIFName,
		TypeIGMPType,
		TypeTimeDate,
		TypeTimeHour,
		TypeTimeDay,
	}
)

// ErrTooManyTypes is the error returned by ConcatSetType, if nftMagic would overflow.
var ErrTooManyTypes = errors.New("too many types to concat")

// MustConcatSetType does the same as ConcatSetType, but panics instead of an
// error. It simplifies safe initialization of global variables.
func MustConcatSetType(types ...SetDatatype) SetDatatype {
	t, err := ConcatSetType(types...)
	if err != nil {
		panic(err)
	}
	return t
}

// ConcatSetType constructs a new SetDatatype which consists of a concatenation
// of the passed types. It returns ErrTooManyTypes, if nftMagic would overflow
// (more than 5 types).
func ConcatSetType(types ...SetDatatype) (SetDatatype, error) {
	if len(types) > 32/SetConcatTypeBits {
		return SetDatatype{}, ErrTooManyTypes
	}

	var magic, bytes uint32
	names := make([]string, len(types))
	for i, t := range types {
		bytes += t.Bytes
		// concatenated types pad the length to multiples of the register size (4 bytes)
		// see https://git.netfilter.org/nftables/tree/src/datatype.c?id=488356b895024d0944b20feb1f930558726e0877#n1162
		if t.Bytes%4 != 0 {
			bytes += 4 - (t.Bytes % 4)
		}
		names[i] = t.Name

		magic <<= SetConcatTypeBits
		magic |= t.nftMagic & SetConcatTypeMask
	}
	return SetDatatype{Name: strings.Join(names, " . "), Bytes: bytes, nftMagic: magic}, nil
}

// Set represents an nftables set. Anonymous sets are only valid within the
// context of a single batch.
type Set struct {
	Table      *Table
	ID         uint32
	Name       string
	Anonymous  bool
	Constant   bool
	Interval   bool
	IsMap      bool
	HasTimeout bool
	Timeout    time.Duration
	KeyType    SetDatatype
	DataType   SetDatatype
}

// SetElement represents a data point within a set.
type SetElement struct {
	Key         []byte
	Val         []byte
	IntervalEnd bool
	// To support vmap, a caller must be able to pass Verdict type of data.
	// If IsMap is true and VerdictData is not nil, then Val of SetElement will be ignored
	// and VerdictData will be wrapped into Attribute data.
	VerdictData *expr.Verdict
	// To support aging of set elements
	Timeout time.Duration
}

func (s *SetElement) decode() func(b []byte) error {
	return func(b []byte) error {
		ad, err := netlink.NewAttributeDecoder(b)
		if err != nil {
			return fmt.Errorf("failed to create nested attribute decoder: %v", err)
		}
		ad.ByteOrder = binary.BigEndian

		for ad.Next() {
			switch ad.Type() {
			case unix.NFTA_SET_ELEM_KEY:
				s.Key, err = decodeElement(ad.Bytes())
				if err != nil {
					return err
				}
			case unix.NFTA_SET_ELEM_DATA:
				s.Val, err = decodeElement(ad.Bytes())
				if err != nil {
					return err
				}
			case unix.NFTA_SET_ELEM_FLAGS:
				flags := ad.Uint32()
				s.IntervalEnd = (flags & unix.NFT_SET_ELEM_INTERVAL_END) != 0
			case unix.NFTA_SET_ELEM_TIMEOUT:
				s.Timeout = time.Duration(time.Millisecond * time.Duration(ad.Uint64()))
			}
		}
		return ad.Err()
	}
}

func decodeElement(d []byte) ([]byte, error) {
	ad, err := netlink.NewAttributeDecoder(d)
	if err != nil {
		return nil, fmt.Errorf("failed to create nested attribute decoder: %v", err)
	}
	ad.ByteOrder = binary.BigEndian
	var b []byte
	for ad.Next() {
		switch ad.Type() {
		case unix.NFTA_SET_ELEM_KEY:
			fallthrough
		case unix.NFTA_SET_ELEM_DATA:
			b = ad.Bytes()
		}
	}
	if err := ad.Err(); err != nil {
		return nil, err
	}
	return b, nil
}

// SetAddElements applies data points to an nftables set.
func (cc *Conn) SetAddElements(s *Set, vals []SetElement) error {
	cc.Lock()
	defer cc.Unlock()
	if s.Anonymous {
		return errors.New("anonymous sets cannot be updated")
	}

	elements, err := s.makeElemList(vals, s.ID)
	if err != nil {
		return err
	}
	cc.messages = append(cc.messages, netlink.Message{
		Header: netlink.Header{
			Type:  netlink.HeaderType((unix.NFNL_SUBSYS_NFTABLES << 8) | unix.NFT_MSG_NEWSETELEM),
			Flags: netlink.Request | netlink.Acknowledge | netlink.Create,
		},
		Data: append(extraHeader(uint8(s.Table.Family), 0), cc.marshalAttr(elements)...),
	})

	return nil
}

func (s *Set) makeElemList(vals []SetElement, id uint32) ([]netlink.Attribute, error) {
	var elements []netlink.Attribute

	for i, v := range vals {
		item := make([]netlink.Attribute, 0)
		var flags uint32
		if v.IntervalEnd {
			flags |= unix.NFT_SET_ELEM_INTERVAL_END
			item = append(item, netlink.Attribute{Type: unix.NFTA_SET_ELEM_FLAGS | unix.NLA_F_NESTED, Data: binaryutil.BigEndian.PutUint32(flags)})
		}

		encodedKey, err := netlink.MarshalAttributes([]netlink.Attribute{{Type: unix.NFTA_DATA_VALUE, Data: v.Key}})
		if err != nil {
			return nil, fmt.Errorf("marshal key %d: %v", i, err)
		}
		item = append(item, netlink.Attribute{Type: unix.NFTA_SET_ELEM_KEY | unix.NLA_F_NESTED, Data: encodedKey})
		if s.HasTimeout && v.Timeout != 0 {
			// Set has Timeout flag set, which means an individual element can specify its own timeout.
			item = append(item, netlink.Attribute{Type: unix.NFTA_SET_ELEM_TIMEOUT, Data: binaryutil.BigEndian.PutUint64(uint64(v.Timeout.Milliseconds()))})
		}
		// The following switch statement deal with 3 different types of elements.
		// 1. v is an element of vmap
		// 2. v is an element of a regular map
		// 3. v is an element of a regular set (default)
		switch {
		case v.VerdictData != nil:
			// Since VerdictData is not nil, v is vmap element, need to add to the attributes
			encodedVal := []byte{}
			encodedKind, err := netlink.MarshalAttributes([]netlink.Attribute{
				{Type: unix.NFTA_DATA_VALUE, Data: binaryutil.BigEndian.PutUint32(uint32(v.VerdictData.Kind))},
			})
			if err != nil {
				return nil, fmt.Errorf("marshal item %d: %v", i, err)
			}
			encodedVal = append(encodedVal, encodedKind...)
			if len(v.VerdictData.Chain) != 0 {
				encodedChain, err := netlink.MarshalAttributes([]netlink.Attribute{
					{Type: unix.NFTA_SET_ELEM_DATA, Data: []byte(v.VerdictData.Chain + "\x00")},
				})
				if err != nil {
					return nil, fmt.Errorf("marshal item %d: %v", i, err)
				}
				encodedVal = append(encodedVal, encodedChain...)
			}
			encodedVerdict, err := netlink.MarshalAttributes([]netlink.Attribute{
				{Type: unix.NFTA_SET_ELEM_DATA | unix.NLA_F_NESTED, Data: encodedVal}})
			if err != nil {
				return nil, fmt.Errorf("marshal item %d: %v", i, err)
			}
			item = append(item, netlink.Attribute{Type: unix.NFTA_SET_ELEM_DATA | unix.NLA_F_NESTED, Data: encodedVerdict})
		case len(v.Val) > 0:
			// Since v.Val's length is not 0 then, v is a regular map element, need to add to the attributes
			encodedVal, err := netlink.MarshalAttributes([]netlink.Attribute{{Type: unix.NFTA_DATA_VALUE, Data: v.Val}})
			if err != nil {
				return nil, fmt.Errorf("marshal item %d: %v", i, err)
			}

			item = append(item, netlink.Attribute{Type: unix.NFTA_SET_ELEM_DATA | unix.NLA_F_NESTED, Data: encodedVal})
		default:
			// If niether of previous cases matche, it means 'e' is an element of a regular Set, no need to add to the attributes
		}

		encodedItem, err := netlink.MarshalAttributes(item)
		if err != nil {
			return nil, fmt.Errorf("marshal item %d: %v", i, err)
		}
		elements = append(elements, netlink.Attribute{Type: uint16(i+1) | unix.NLA_F_NESTED, Data: encodedItem})
	}

	encodedElem, err := netlink.MarshalAttributes(elements)
	if err != nil {
		return nil, fmt.Errorf("marshal elements: %v", err)
	}

	return []netlink.Attribute{
		{Type: unix.NFTA_SET_NAME, Data: []byte(s.Name + "\x00")},
		{Type: unix.NFTA_LOOKUP_SET_ID, Data: binaryutil.BigEndian.PutUint32(id)},
		{Type: unix.NFTA_SET_TABLE, Data: []byte(s.Table.Name + "\x00")},
		{Type: unix.NFTA_SET_ELEM_LIST_ELEMENTS | unix.NLA_F_NESTED, Data: encodedElem},
	}, nil
}

// AddSet adds the specified Set.
func (cc *Conn) AddSet(s *Set, vals []SetElement) error {
	cc.Lock()
	defer cc.Unlock()
	// Based on nft implementation & linux source.
	// Link: https://github.com/torvalds/linux/blob/49a57857aeea06ca831043acbb0fa5e0f50602fd/net/netfilter/nf_tables_api.c#L3395
	// Another reference: https://git.netfilter.org/nftables/tree/src

	if s.Anonymous && !s.Constant {
		return errors.New("anonymous structs must be constant")
	}

	if s.ID == 0 {
		allocSetID++
		s.ID = allocSetID
		if s.Anonymous {
			s.Name = "__set%d"
			if s.IsMap {
				s.Name = "__map%d"
			}
		}
	}

	var flags uint32
	if s.Anonymous {
		flags |= unix.NFT_SET_ANONYMOUS
	}
	if s.Constant {
		flags |= unix.NFT_SET_CONSTANT
	}
	if s.Interval {
		flags |= unix.NFT_SET_INTERVAL
	}
	if s.IsMap {
		flags |= unix.NFT_SET_MAP
	}
	if s.HasTimeout {
		flags |= unix.NFT_SET_TIMEOUT
	}
	tableInfo := []netlink.Attribute{
		{Type: unix.NFTA_SET_TABLE, Data: []byte(s.Table.Name + "\x00")},
		{Type: unix.NFTA_SET_NAME, Data: []byte(s.Name + "\x00")},
		{Type: unix.NFTA_SET_FLAGS, Data: binaryutil.BigEndian.PutUint32(flags)},
		{Type: unix.NFTA_SET_KEY_TYPE, Data: binaryutil.BigEndian.PutUint32(s.KeyType.nftMagic)},
		{Type: unix.NFTA_SET_KEY_LEN, Data: binaryutil.BigEndian.PutUint32(s.KeyType.Bytes)},
		{Type: unix.NFTA_SET_ID, Data: binaryutil.BigEndian.PutUint32(s.ID)},
	}
	if s.IsMap {
		// Check if it is vmap case
		if s.DataType.nftMagic == 1 {
			// For Verdict data type, the expected magic is 0xfffff0
			tableInfo = append(tableInfo, netlink.Attribute{Type: unix.NFTA_SET_DATA_TYPE, Data: binaryutil.BigEndian.PutUint32(uint32(unix.NFT_DATA_VERDICT))},
				netlink.Attribute{Type: unix.NFTA_SET_DATA_LEN, Data: binaryutil.BigEndian.PutUint32(s.DataType.Bytes)})
		} else {
			tableInfo = append(tableInfo, netlink.Attribute{Type: unix.NFTA_SET_DATA_TYPE, Data: binaryutil.BigEndian.PutUint32(s.DataType.nftMagic)},
				netlink.Attribute{Type: unix.NFTA_SET_DATA_LEN, Data: binaryutil.BigEndian.PutUint32(s.DataType.Bytes)})
		}
	}
	if s.HasTimeout && s.Timeout != 0 {
		// If Set's global timeout is specified, add it to set's attributes
		tableInfo = append(tableInfo, netlink.Attribute{Type: unix.NFTA_SET_TIMEOUT, Data: binaryutil.BigEndian.PutUint64(uint64(s.Timeout.Milliseconds()))})
	}
	if s.Constant {
		// nft cli tool adds the number of elements to set/map's descriptor
		// It make sense to do only if a set or map are constant, otherwise skip NFTA_SET_DESC attribute
		numberOfElements, err := netlink.MarshalAttributes([]netlink.Attribute{
			{Type: unix.NFTA_DATA_VALUE, Data: binaryutil.BigEndian.PutUint32(uint32(len(vals)))},
		})
		if err != nil {
			return fmt.Errorf("fail to marshal number of elements %d: %v", len(vals), err)
		}
		tableInfo = append(tableInfo, netlink.Attribute{Type: unix.NLA_F_NESTED | unix.NFTA_SET_DESC, Data: numberOfElements})
	}

	if s.Anonymous || s.Constant || s.Interval {
		tableInfo = append(tableInfo,
			// Semantically useless - kept for binary compatability with nft
			netlink.Attribute{Type: unix.NFTA_SET_USERDATA, Data: []byte("\x00\x04\x02\x00\x00\x00")})
	}

	cc.messages = append(cc.messages, netlink.Message{
		Header: netlink.Header{
			Type:  netlink.HeaderType((unix.NFNL_SUBSYS_NFTABLES << 8) | unix.NFT_MSG_NEWSET),
			Flags: netlink.Request | netlink.Acknowledge | netlink.Create,
		},
		Data: append(extraHeader(uint8(s.Table.Family), 0), cc.marshalAttr(tableInfo)...),
	})

	// Set the values of the set if initial values were provided.
	if len(vals) > 0 {
		hdrType := unix.NFT_MSG_NEWSETELEM
		elements, err := s.makeElemList(vals, s.ID)
		if err != nil {
			return err
		}
		cc.messages = append(cc.messages, netlink.Message{
			Header: netlink.Header{
				Type:  netlink.HeaderType((unix.NFNL_SUBSYS_NFTABLES << 8) | hdrType),
				Flags: netlink.Request | netlink.Acknowledge | netlink.Create,
			},
			Data: append(extraHeader(uint8(s.Table.Family), 0), cc.marshalAttr(elements)...),
		})
	}

	return nil
}

// DelSet deletes a specific set, along with all elements it contains.
func (cc *Conn) DelSet(s *Set) {
	cc.Lock()
	defer cc.Unlock()
	data := cc.marshalAttr([]netlink.Attribute{
		{Type: unix.NFTA_SET_TABLE, Data: []byte(s.Table.Name + "\x00")},
		{Type: unix.NFTA_SET_NAME, Data: []byte(s.Name + "\x00")},
	})
	cc.messages = append(cc.messages, netlink.Message{
		Header: netlink.Header{
			Type:  netlink.HeaderType((unix.NFNL_SUBSYS_NFTABLES << 8) | unix.NFT_MSG_DELSET),
			Flags: netlink.Request | netlink.Acknowledge,
		},
		Data: append(extraHeader(uint8(s.Table.Family), 0), data...),
	})
}

// SetDeleteElements deletes data points from an nftables set.
func (cc *Conn) SetDeleteElements(s *Set, vals []SetElement) error {
	cc.Lock()
	defer cc.Unlock()
	if s.Anonymous {
		return errors.New("anonymous sets cannot be updated")
	}

	elements, err := s.makeElemList(vals, s.ID)
	if err != nil {
		return err
	}
	cc.messages = append(cc.messages, netlink.Message{
		Header: netlink.Header{
			Type:  netlink.HeaderType((unix.NFNL_SUBSYS_NFTABLES << 8) | unix.NFT_MSG_DELSETELEM),
			Flags: netlink.Request | netlink.Acknowledge | netlink.Create,
		},
		Data: append(extraHeader(uint8(s.Table.Family), 0), cc.marshalAttr(elements)...),
	})

	return nil
}

// FlushSet deletes all data points from an nftables set.
func (cc *Conn) FlushSet(s *Set) {
	cc.Lock()
	defer cc.Unlock()
	data := cc.marshalAttr([]netlink.Attribute{
		{Type: unix.NFTA_SET_TABLE, Data: []byte(s.Table.Name + "\x00")},
		{Type: unix.NFTA_SET_NAME, Data: []byte(s.Name + "\x00")},
	})
	cc.messages = append(cc.messages, netlink.Message{
		Header: netlink.Header{
			Type:  netlink.HeaderType((unix.NFNL_SUBSYS_NFTABLES << 8) | unix.NFT_MSG_DELSETELEM),
			Flags: netlink.Request | netlink.Acknowledge,
		},
		Data: append(extraHeader(uint8(s.Table.Family), 0), data...),
	})
}

var setHeaderType = netlink.HeaderType((unix.NFNL_SUBSYS_NFTABLES << 8) | unix.NFT_MSG_NEWSET)

func setsFromMsg(msg netlink.Message) (*Set, error) {
	if got, want := msg.Header.Type, setHeaderType; got != want {
		return nil, fmt.Errorf("unexpected header type: got %v, want %v", got, want)
	}
	ad, err := netlink.NewAttributeDecoder(msg.Data[4:])
	if err != nil {
		return nil, err
	}
	ad.ByteOrder = binary.BigEndian

	var set Set
	for ad.Next() {
		switch ad.Type() {
		case unix.NFTA_SET_NAME:
			set.Name = ad.String()
		case unix.NFTA_SET_ID:
			set.ID = binary.BigEndian.Uint32(ad.Bytes())
		case unix.NFTA_SET_TIMEOUT:
			set.Timeout = time.Duration(time.Millisecond * time.Duration(binary.BigEndian.Uint64(ad.Bytes())))
			set.HasTimeout = true
		case unix.NFTA_SET_FLAGS:
			flags := ad.Uint32()
			set.Constant = (flags & unix.NFT_SET_CONSTANT) != 0
			set.Anonymous = (flags & unix.NFT_SET_ANONYMOUS) != 0
			set.Interval = (flags & unix.NFT_SET_INTERVAL) != 0
			set.IsMap = (flags & unix.NFTA_SET_TABLE) != 0
			set.HasTimeout = (flags & unix.NFTA_SET_TIMEOUT) != 0
		case unix.NFTA_SET_KEY_TYPE:
			nftMagic := ad.Uint32()
			if invalidMagic, ok := validateKeyType(nftMagic); !ok {
				return nil, fmt.Errorf("could not determine key type %+v", invalidMagic)
			}
			set.KeyType.nftMagic = nftMagic
		case unix.NFTA_SET_DATA_TYPE:
			nftMagic := ad.Uint32()
			// Special case for the data type verdict, in the message it is stored as 0xffffff00 but it is defined as 1
			if nftMagic == 0xffffff00 {
				set.KeyType = TypeVerdict
				break
			}
			for _, dt := range nftDatatypes {
				if nftMagic == dt.nftMagic {
					set.DataType = dt
					break
				}
			}
			if set.DataType.nftMagic == 0 {
				return nil, fmt.Errorf("could not determine data type %x", nftMagic)
			}
		}
	}
	return &set, nil
}

func validateKeyType(bits uint32) ([]uint32, bool) {
	var unpackTypes []uint32
	var invalidTypes []uint32
	found := false
	valid := true
	for bits != 0 {
		unpackTypes = append(unpackTypes, bits&SetConcatTypeMask)
		bits = bits >> SetConcatTypeBits
	}
	for _, t := range unpackTypes {
		for _, dt := range nftDatatypes {
			if t == dt.nftMagic {
				found = true
			}
		}
		if !found {
			invalidTypes = append(invalidTypes, t)
			valid = false
		}
		found = false
	}
	return invalidTypes, valid
}

var elemHeaderType = netlink.HeaderType((unix.NFNL_SUBSYS_NFTABLES << 8) | unix.NFT_MSG_NEWSETELEM)

func elementsFromMsg(msg netlink.Message) ([]SetElement, error) {
	if got, want := msg.Header.Type, elemHeaderType; got != want {
		return nil, fmt.Errorf("unexpected header type: got %v, want %v", got, want)
	}
	ad, err := netlink.NewAttributeDecoder(msg.Data[4:])
	if err != nil {
		return nil, err
	}
	ad.ByteOrder = binary.BigEndian

	var elements []SetElement
	for ad.Next() {
		b := ad.Bytes()
		if ad.Type() == unix.NFTA_SET_ELEM_LIST_ELEMENTS {
			ad, err := netlink.NewAttributeDecoder(b)
			if err != nil {
				return nil, err
			}
			ad.ByteOrder = binary.BigEndian

			for ad.Next() {
				var elem SetElement
				switch ad.Type() {
				case unix.NFTA_LIST_ELEM:
					ad.Do(elem.decode())
				}
				elements = append(elements, elem)
			}
		}
	}
	return elements, nil
}

// GetSets returns the sets in the specified table.
func (cc *Conn) GetSets(t *Table) ([]*Set, error) {
	conn, err := cc.dialNetlink()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	data, err := netlink.MarshalAttributes([]netlink.Attribute{
		{Type: unix.NFTA_SET_TABLE, Data: []byte(t.Name + "\x00")},
	})
	if err != nil {
		return nil, err
	}

	message := netlink.Message{
		Header: netlink.Header{
			Type:  netlink.HeaderType((unix.NFNL_SUBSYS_NFTABLES << 8) | unix.NFT_MSG_GETSET),
			Flags: netlink.Request | netlink.Acknowledge | netlink.Dump,
		},
		Data: append(extraHeader(uint8(t.Family), 0), data...),
	}

	if _, err := conn.SendMessages([]netlink.Message{message}); err != nil {
		return nil, fmt.Errorf("SendMessages: %v", err)
	}

	reply, err := conn.Receive()
	if err != nil {
		return nil, fmt.Errorf("Receive: %v", err)
	}
	var sets []*Set
	for _, msg := range reply {
		s, err := setsFromMsg(msg)
		if err != nil {
			return nil, err
		}
		s.Table = &Table{Name: t.Name, Use: t.Use, Flags: t.Flags, Family: t.Family}
		sets = append(sets, s)
	}

	return sets, nil
}

// GetSetByName returns the set in the specified table if matching name is found.
func (cc *Conn) GetSetByName(t *Table, name string) (*Set, error) {
	conn, err := cc.dialNetlink()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	data, err := netlink.MarshalAttributes([]netlink.Attribute{
		{Type: unix.NFTA_SET_TABLE, Data: []byte(t.Name + "\x00")},
		{Type: unix.NFTA_SET_NAME, Data: []byte(name + "\x00")},
	})
	if err != nil {
		return nil, err
	}

	message := netlink.Message{
		Header: netlink.Header{
			Type:  netlink.HeaderType((unix.NFNL_SUBSYS_NFTABLES << 8) | unix.NFT_MSG_GETSET),
			Flags: netlink.Request | netlink.Acknowledge,
		},
		Data: append(extraHeader(uint8(t.Family), 0), data...),
	}

	if _, err := conn.SendMessages([]netlink.Message{message}); err != nil {
		return nil, fmt.Errorf("SendMessages: %w", err)
	}

	reply, err := conn.Receive()
	if err != nil {
		return nil, fmt.Errorf("Receive: %w", err)
	}

	if len(reply) != 1 {
		return nil, fmt.Errorf("Receive: expected to receive 1 message but got %d", len(reply))
	}
	rs, err := setsFromMsg(reply[0])
	if err != nil {
		return nil, err
	}
	rs.Table = &Table{Name: t.Name, Use: t.Use, Flags: t.Flags, Family: t.Family}

	return rs, nil
}

// GetSetElements returns the elements in the specified set.
func (cc *Conn) GetSetElements(s *Set) ([]SetElement, error) {
	conn, err := cc.dialNetlink()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	data, err := netlink.MarshalAttributes([]netlink.Attribute{
		{Type: unix.NFTA_SET_TABLE, Data: []byte(s.Table.Name + "\x00")},
		{Type: unix.NFTA_SET_NAME, Data: []byte(s.Name + "\x00")},
	})
	if err != nil {
		return nil, err
	}

	message := netlink.Message{
		Header: netlink.Header{
			Type:  netlink.HeaderType((unix.NFNL_SUBSYS_NFTABLES << 8) | unix.NFT_MSG_GETSETELEM),
			Flags: netlink.Request | netlink.Acknowledge | netlink.Dump,
		},
		Data: append(extraHeader(uint8(s.Table.Family), 0), data...),
	}

	if _, err := conn.SendMessages([]netlink.Message{message}); err != nil {
		return nil, fmt.Errorf("SendMessages: %v", err)
	}

	reply, err := conn.Receive()
	if err != nil {
		return nil, fmt.Errorf("Receive: %v", err)
	}
	var elems []SetElement
	for _, msg := range reply {
		s, err := elementsFromMsg(msg)
		if err != nil {
			return nil, err
		}
		elems = append(elems, s...)
	}

	return elems, nil
}
