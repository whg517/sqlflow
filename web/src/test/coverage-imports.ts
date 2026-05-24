// Barrel import to ensure all source modules are instrumented by V8 coverage
// This file exists solely to import modules so V8 tracks them

// API modules
import "@/api/audit";
import "@/api/comment";
import "@/api/dashboard";
import "@/api/maskRule";
import "@/api/query";
import "@/api/ticket";
import "@/api/user";
import "@/api/client";

// Components
import "@/components/ErrorPage";
import "@/components/NetworkBanner";
import "@/components/SensitiveTableBadge";

// Hooks
import "@/hooks/useSensitiveTables";

// Pages
import "@/pages/Audit";
import "@/pages/Dashboard";
import "@/pages/Permissions";
import "@/pages/Query";
import "@/pages/Settings";
import "@/pages/Ticket";
import "@/pages/TicketNew";
import "@/pages/Users";
import "@/pages/Query/components/AIReviewCard";
import "@/pages/Query/components/HistoryPanel";
import "@/pages/Query/components/MongoEditor";
import "@/pages/Query/components/QueryTabs";
import "@/pages/Query/components/ResizableSplit";
import "@/pages/Query/components/SqlEditor";
import "@/pages/Query/components/StatusBar";
import "@/pages/Query/components/TicketSubmitSheet";
import "@/pages/Ticket/components/CommentSection";
import "@/pages/Ticket/components/TicketDetailDrawer";
import "@/pages/Settings/AIConfigTab";
import "@/pages/Settings/MaskRulesTab";
