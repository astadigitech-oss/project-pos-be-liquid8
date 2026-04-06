package response

type ResponseResource struct {
	Status  bool        `json:"status"`
	Message string      `json:"message"`
	Resource    interface{} `json:"resource,omitempty"`
}

func Success(message string, resource interface{}) ResponseResource {
	return ResponseResource{
		Status:  true,
		Message: message,
		Resource:  resource,
	}
}

func Error(message string) ResponseResource {
	return ResponseResource{
		Status:  false,
		Message: message,
	}
}
