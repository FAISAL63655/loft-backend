// Package users provides user profile and address management services
package users

import "encore.app/pkg/errs"

// User-related errors
var (
	ErrUserNotFound = &errs.Error{
		Code:    errs.NotFound,
		Message: "المستخدم غير موجود.",
	}

	ErrUserInactive = &errs.Error{
		Code:    errs.PermissionDenied,
		Message: "حسابك غير نشط. يرجى التواصل مع الدعم.",
	}

	ErrEmailNotVerified = &errs.Error{
		Code:    errs.FailedPrecondition,
		Message: "يرجى تفعيل بريدك الإلكتروني لإتمام هذه العملية.",
	}

	ErrInvalidCity = &errs.Error{
		Code:    errs.InvalidArgument,
		Message: "المدينة المحددة غير صالحة.",
	}
)

// Verification-related errors
var (
	ErrAlreadyVerified = &errs.Error{
		Code:    errs.FailedPrecondition,
		Message: "حسابك موثّق مسبقًا.",
	}

	ErrVerificationPending = &errs.Error{
		Code:    errs.FailedPrecondition,
		Message: "لديك طلب توثيق معلق بالفعل.",
	}

	ErrVerificationNotFound = &errs.Error{
		Code:    errs.NotFound,
		Message: "طلب التوثيق غير موجود.",
	}

	ErrVerificationNotPending = &errs.Error{
		Code:    errs.FailedPrecondition,
		Message: "طلب التوثيق ليس في حالة انتظار.",
	}

	ErrInsufficientPermissions = &errs.Error{
		Code:    errs.PermissionDenied,
		Message: "لا تملك صلاحية لتنفيذ هذا الإجراء.",
	}
)

// Address-related errors
var (
	ErrAddressNotFound = &errs.Error{
		Code:    errs.NotFound,
		Message: "العنوان غير موجود.",
	}

	ErrAddressPermissionDenied = &errs.Error{
		Code:    errs.PermissionDenied,
		Message: "لا تملك صلاحية لتعديل هذا العنوان.",
	}

	ErrDefaultAddressExists = &errs.Error{
		Code:    errs.FailedPrecondition,
		Message: "لديك عنوان افتراضي بالفعل. يرجى إلغاء الافتراضي الحالي أولاً.",
	}
)

// Database-related errors
var (
	ErrDatabaseQuery = &errs.Error{
		Code:    errs.Internal,
		Message: "خطأ في استعلام قاعدة البيانات.",
	}

	ErrTransactionFailed = &errs.Error{
		Code:    errs.Internal,
		Message: "فشل في المعاملة.",
	}
)

// Success messages
const (
	MsgProfileUpdated        = "تم تحديث الملف الشخصي بنجاح."
	MsgVerificationRequested = "تم استلام طلب توثيق حسابك."
	MsgVerificationApproved  = "تمت الموافقة على طلب التوثيق وتمت ترقية دورك."
	MsgVerificationRejected  = "تم رفض طلب التوثيق."
	MsgAddressCreated        = "تم إنشاء العنوان بنجاح."
	MsgAddressUpdated        = "تم تحديث العنوان بنجاح."
	MsgAddressArchived       = "تم أرشفة العنوان بنجاح."
)
