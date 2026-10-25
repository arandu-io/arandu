//go:build kyse

// Example resource. Remove with the list under "The example resource" in README.md.

package partials

import (
	"github.com/arandu-io/kyse/components"

	comments "github.com/arandu-io/arandu/storage/framework/views/comments"
)

@go
// CommentsTableData is what the comments listing's table draws.
//
// It is the listing page's own data. The page draws this file with @include,
// which hands its data over unchanged, and CommentController.Index fills the
// same struct when it answers the table alone, so both draw from one template.
type CommentsTableData = comments.CommentsIndexData
@endgo

{{-- The table alone: its root element is the region every control swaps. The
     controller answers a request that names it in HX-Target with this file and
     nothing around it, and every other request to the same address with the
     whole page. The table draws its own pager, and there is no other. --}}
{!! components.DataTable(comments.CommentIndexTable(.)) !!}
