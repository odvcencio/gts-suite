package capa

// BuiltinRules returns the default set of capability detection rules.
func BuiltinRules() []Rule {
	return []Rule{
		{
			Name: "Encrypt/Decrypt Data", Description: "Uses cryptographic APIs",
			AttackID: "T1027", Category: "crypto",
			AnyAPIs: []string{"CryptEncrypt", "CryptDecrypt", "BCryptEncrypt", "BCryptDecrypt",
				"EVP_EncryptInit", "EVP_DecryptInit", "AES_encrypt", "AES_decrypt",
				"SHA256_Init", "SHA256_Update", "MD5_Init"},
			Confidence: "high",
		},
		{
			Name: "Network Communication", Description: "Uses network socket or HTTP APIs",
			AttackID: "T1071", Category: "network",
			AnyAPIs: []string{"connect", "send", "recv", "WSAStartup", "WSASocket",
				"InternetOpenA", "InternetOpenW", "HttpSendRequestA", "HttpSendRequestW",
				"HttpOpenRequestA", "socket", "bind", "listen", "accept",
				"getaddrinfo", "gethostbyname"},
			Confidence: "high",
		},
		{
			Name: "Process Injection", Description: "Classic VirtualAllocEx + WriteProcessMemory + CreateRemoteThread injection",
			AttackID: "T1055", Category: "process_injection",
			AllAPIs:    []string{"VirtualAllocEx", "WriteProcessMemory", "CreateRemoteThread"},
			Confidence: "high",
		},
		{
			Name: "Registry Persistence", Description: "Writes to registry for persistence",
			AttackID: "T1547", Category: "persistence_registry",
			AnyAPIs:    []string{"RegSetValueExA", "RegSetValueExW", "RegCreateKeyExA", "RegCreateKeyExW"},
			Confidence: "medium",
		},
		{
			Name: "Anti-Debug Checks", Description: "Checks for debugger presence",
			AttackID: "T1622", Category: "anti_debug",
			AnyAPIs:    []string{"IsDebuggerPresent", "CheckRemoteDebuggerPresent", "NtQueryInformationProcess"},
			Confidence: "high",
		},
		{
			Name: "Dynamic API Resolution", Description: "Resolves APIs at runtime",
			AttackID: "T1106", Category: "dynamic_api",
			AnyAPIs:    []string{"GetProcAddress", "LoadLibraryA", "LoadLibraryW", "LdrLoadDll"},
			Confidence: "medium",
		},
		{
			Name: "File Operations", Description: "Creates, writes, or deletes files",
			AttackID: "T1070", Category: "file_ops",
			AnyAPIs:    []string{"CreateFileA", "CreateFileW", "WriteFile", "DeleteFileA", "DeleteFileW", "MoveFileA"},
			Confidence: "low",
		},
	}
}
