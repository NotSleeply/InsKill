package model

import "time"

// UserDTO 登录态下发给前端/存 Redis 的用户精简信息，字段对齐 Java UserDTO。
type UserDTO struct {
	ID       int64  `json:"id" gorm:"column:id"`
	NickName string `json:"nickName" gorm:"column:nick_name"`
	Icon     string `json:"icon" gorm:"column:icon"`
}

type User struct {
	ID         int64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	Phone      string    `json:"phone" gorm:"column:phone"`
	Password   string    `json:"-" gorm:"column:password"`
	NickName   string    `json:"nickName" gorm:"column:nick_name"`
	Icon       string    `json:"icon" gorm:"column:icon"`
	CreateTime time.Time `json:"-" gorm:"column:create_time;default:CURRENT_TIMESTAMP"`
	UpdateTime time.Time `json:"-" gorm:"column:update_time;default:CURRENT_TIMESTAMP"`
}

func (User) TableName() string { return "user" }

type UserInfo struct {
	UserID     int64      `json:"userId" gorm:"column:user_id;primaryKey"`
	City       string     `json:"city" gorm:"column:city"`
	Introduce  string     `json:"introduce" gorm:"column:introduce"`
	Fans       int32      `json:"fans" gorm:"column:fans"`
	Followee   int32      `json:"followee" gorm:"column:followee"`
	Gender     bool       `json:"gender" gorm:"column:gender"`
	Birthday   *time.Time `json:"birthday" gorm:"column:birthday"`
	Credits    int32      `json:"credits" gorm:"column:credits"`
	Level      bool       `json:"level" gorm:"column:level"`
	CreateTime time.Time  `json:"-" gorm:"column:create_time;default:CURRENT_TIMESTAMP"`
	UpdateTime time.Time  `json:"-" gorm:"column:update_time;default:CURRENT_TIMESTAMP"`
}

func (UserInfo) TableName() string { return "user_info" }

type Shop struct {
	ID         int64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	Name       string    `json:"name" gorm:"column:name"`
	TypeID     int64     `json:"typeId" gorm:"column:type_id"`
	Images     string    `json:"images" gorm:"column:images"`
	Area       string    `json:"area" gorm:"column:area"`
	Address    string    `json:"address" gorm:"column:address"`
	X          float64   `json:"x" gorm:"column:x"`
	Y          float64   `json:"y" gorm:"column:y"`
	AvgPrice   int64     `json:"avgPrice" gorm:"column:avg_price"`
	Sold       int32     `json:"sold" gorm:"column:sold"`
	Comments   int32     `json:"comments" gorm:"column:comments"`
	Score      int32     `json:"score" gorm:"column:score"`
	OpenHours  string    `json:"openHours" gorm:"column:open_hours"`
	Distance   float64   `json:"distance,omitempty" gorm:"-"`
	CreateTime time.Time `json:"-" gorm:"column:create_time;default:CURRENT_TIMESTAMP"`
	UpdateTime time.Time `json:"-" gorm:"column:update_time;default:CURRENT_TIMESTAMP"`
}

func (Shop) TableName() string { return "shop" }

type ShopType struct {
	ID         int64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	Name       string    `json:"name" gorm:"column:name"`
	Icon       string    `json:"icon" gorm:"column:icon"`
	Sort       int32     `json:"sort" gorm:"column:sort"`
	CreateTime time.Time `json:"-" gorm:"column:create_time;default:CURRENT_TIMESTAMP"`
	UpdateTime time.Time `json:"-" gorm:"column:update_time;default:CURRENT_TIMESTAMP"`
}

func (ShopType) TableName() string { return "shop_type" }

type Voucher struct {
	ID          int64      `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	ShopID      int64      `json:"shopId" gorm:"column:shop_id"`
	Title       string     `json:"title" gorm:"column:title"`
	SubTitle    string     `json:"subTitle" gorm:"column:sub_title"`
	Rules       string     `json:"rules" gorm:"column:rules"`
	PayValue    int64      `json:"payValue" gorm:"column:pay_value"`
	ActualValue int64      `json:"actualValue" gorm:"column:actual_value"`
	Type        int8       `json:"type" gorm:"column:type"`
	Status      int8       `json:"status" gorm:"column:status"`
	Stock       int32      `json:"stock,omitempty" gorm:"column:stock"`
	BeginTime   *time.Time `json:"beginTime,omitempty" gorm:"column:begin_time"`
	EndTime     *time.Time `json:"endTime,omitempty" gorm:"column:end_time"`
	CreateTime  time.Time  `json:"-" gorm:"column:create_time;default:CURRENT_TIMESTAMP"`
	UpdateTime  time.Time  `json:"-" gorm:"column:update_time;default:CURRENT_TIMESTAMP"`
}

func (Voucher) TableName() string { return "voucher" }

type SeckillVoucher struct {
	VoucherID  int64     `json:"voucherId" gorm:"column:voucher_id;primaryKey"`
	Stock      int32     `json:"stock" gorm:"column:stock"`
	CreateTime time.Time `json:"-" gorm:"column:create_time;default:CURRENT_TIMESTAMP"`
	BeginTime  *time.Time `json:"beginTime,omitempty" gorm:"column:begin_time"`
	EndTime    *time.Time `json:"endTime,omitempty" gorm:"column:end_time"`
	UpdateTime time.Time `json:"-" gorm:"column:update_time;default:CURRENT_TIMESTAMP"`
}

func (SeckillVoucher) TableName() string { return "seckill_voucher" }

type VoucherOrder struct {
	ID         int64      `json:"id" gorm:"column:id;primaryKey"`
	UserID     int64      `json:"userId" gorm:"column:user_id"`
	VoucherID  int64      `json:"voucherId" gorm:"column:voucher_id"`
	PayType    int8       `json:"payType" gorm:"column:pay_type"`
	Status     int8       `json:"status" gorm:"column:status"`
	CreateTime time.Time  `json:"createTime" gorm:"column:create_time;default:CURRENT_TIMESTAMP"`
	PayTime    *time.Time `json:"payTime,omitempty" gorm:"column:pay_time"`
	UseTime    *time.Time `json:"useTime,omitempty" gorm:"column:use_time"`
	RefundTime *time.Time `json:"refundTime,omitempty" gorm:"column:refund_time"`
	UpdateTime time.Time  `json:"-" gorm:"column:update_time;default:CURRENT_TIMESTAMP"`
}

// 订单状态常量
const (
	OrderStatusUnpaid   int8 = 1 // 未支付
	OrderStatusPaid     int8 = 2 // 已支付
	OrderStatusUsed     int8 = 3 // 已核销
	OrderStatusCanceled int8 = 4 // 已取消
	OrderStatusRefund   int8 = 5 // 退款中
	OrderStatusRefunded int8 = 6 // 已退款
)

func (VoucherOrder) TableName() string { return "voucher_order" }

// Blog 探店笔记。Icon/Name/IsLike 为非表字段，查询时填充（对齐 Java @TableField(exist=false)）。
type Blog struct {
	ID         int64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	ShopID     int64     `json:"shopId" gorm:"column:shop_id"`
	UserID     int64     `json:"userId" gorm:"column:user_id"`
	Icon       string    `json:"icon,omitempty" gorm:"-"`
	Name       string    `json:"name,omitempty" gorm:"-"`
	IsLike     bool      `json:"isLike" gorm:"-"`
	Title      string    `json:"title" gorm:"column:title"`
	Images     string    `json:"images" gorm:"column:images"`
	Content    string    `json:"content" gorm:"column:content"`
	Liked      int32     `json:"liked" gorm:"column:liked"`
	Comments   int32     `json:"comments" gorm:"column:comments"`
	CreateTime time.Time `json:"-" gorm:"column:create_time;default:CURRENT_TIMESTAMP"`
	UpdateTime time.Time `json:"-" gorm:"column:update_time;default:CURRENT_TIMESTAMP"`
}

func (Blog) TableName() string { return "blog" }

type BlogComments struct {
	ID         int64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	UserID     int64     `json:"userId" gorm:"column:user_id"`
	BlogID     int64     `json:"blogId" gorm:"column:blog_id"`
	ParentID   int64     `json:"parentId" gorm:"column:parent_id"`
	AnswerID   int64     `json:"answerId" gorm:"column:answer_id"`
	Content    string    `json:"content" gorm:"column:content"`
	Liked      int32     `json:"liked" gorm:"column:liked"`
	Status     int8      `json:"status" gorm:"column:status"`
	CreateTime time.Time `json:"-" gorm:"column:create_time;default:CURRENT_TIMESTAMP"`
	UpdateTime time.Time `json:"-" gorm:"column:update_time;default:CURRENT_TIMESTAMP"`
}

func (BlogComments) TableName() string { return "blog_comments" }

type Follow struct {
	ID           int64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	UserID       int64     `json:"userId" gorm:"column:user_id"`
	FollowUserID int64     `json:"followUserId" gorm:"column:follow_user_id"`
	CreateTime   time.Time `json:"-" gorm:"column:create_time;default:CURRENT_TIMESTAMP"`
}

func (Follow) TableName() string { return "follow" }

// ScrollResult 滚动分页返回结构，字段对齐 Java ScrollResult。
type ScrollResult struct {
	List    []*Blog `json:"list"`
	MinTime int64   `json:"minTime"`
	Offset  int     `json:"offset"`
}
