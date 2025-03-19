//! Memory management module for efficient data handling

use std::collections::HashMap;
use std::sync::{Arc, Mutex};

/// Manages memory allocations and access for algorithms
pub struct MemoryManager {
    // Memory regions accessible by algorithms
    shared_memory: HashMap<String, Vec<u8>>,
    // Protected memory regions that require special access
    protected_memory: Arc<Mutex<HashMap<String, Vec<u8>>>>,
}

impl MemoryManager {
    /// Create a new memory manager instance
    pub fn new() -> Self {
        Self {
            shared_memory: HashMap::new(),
            protected_memory: Arc::new(Mutex::new(HashMap::new())),
        }
    }
    
    /// Allocate memory in the shared region
    pub fn allocate(&mut self, key: &str, size: usize) -> &mut [u8] {
        let buffer = vec![0u8; size];
        self.shared_memory.insert(key.to_string(), buffer);
        self.shared_memory.get_mut(key).unwrap().as_mut_slice()
    }
    
    /// Read data from shared memory
    pub fn read(&self, key: &str) -> Option<&[u8]> {
        self.shared_memory.get(key).map(|data| data.as_slice())
    }
    
    /// Write data to shared memory
    pub fn write(&mut self, key: &str, data: &[u8]) -> Result<(), String> {
        if let Some(buffer) = self.shared_memory.get_mut(key) {
            if buffer.len() >= data.len() {
                buffer[..data.len()].copy_from_slice(data);
                Ok(())
            } else {
                Err("Buffer too small".to_string())
            }
        } else {
            self.shared_memory.insert(key.to_string(), data.to_vec());
            Ok(())
        }
    }
}

impl Default for MemoryManager {
    fn default() -> Self {
        Self::new()
    }
}
