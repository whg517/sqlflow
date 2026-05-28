import { Navigate } from "react-router-dom";

/** Coverage audit entry — redirects to summary view. */
export default function CoveragePage() {
  return <Navigate to="/coverage/summary" replace />;
}
